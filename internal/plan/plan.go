// Package plan selects optimizations for a guest, decides which changes are
// safe to apply without cutting off the Packer WinRM session, compares them
// with the inspected state, verifies the result, and keeps the rollback
// journal. It is pure: all guest input comes in as facts and states.
package plan

import (
	"fmt"
	"slices"
	"strings"

	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/catalog"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/facts"
)

// Status is the outcome for one optimization.
type Status string

const (
	StatusCompliant   Status = "compliant"
	StatusPending     Status = "pending"
	StatusApplied     Status = "applied"
	StatusSkipped     Status = "skipped"
	StatusUnsupported Status = "unsupported"
	StatusFailed      Status = "failed"
)

// ChangeStatus is the outcome for one change.
type ChangeStatus string

const (
	ChangeCompliant     ChangeStatus = "compliant"
	ChangePending       ChangeStatus = "pending"
	ChangeApplied       ChangeStatus = "applied"
	ChangeRestored      ChangeStatus = "restored"
	ChangeNotApplicable ChangeStatus = "not_applicable"
	ChangeSkipped       ChangeStatus = "skipped"
	ChangeFailed        ChangeStatus = "failed"
)

// Options selects optimizations. Profile adds every optimization of that
// profile in Categories (all categories when empty). Include adds
// optimizations by ID pattern regardless of profile and category, but a
// high-risk optimization only by its exact ID. Exclude wins over both.
type Options struct {
	Profile    catalog.Profile
	Categories []catalog.Category
	Include    []string
	Exclude    []string
}

// Item is the plan entry for one selected optimization.
type Item struct {
	Optimization catalog.Optimization
	SelectedBy   string
	Status       Status
	Reason       string
	Changes      []*ChangeItem
}

// ChangeItem is one change with its resolved target. Before is the state found
// by the first inspection, After the state after applying, and Restore the
// value that puts the target back into its Before state.
type ChangeItem struct {
	OptimizationID string
	RebootRequired bool
	Change         catalog.Change
	Before         *State
	After          *State
	Restore        *catalog.Value
	Status         ChangeStatus
	Reason         string
}

// ID returns the target ID of the change.
//
// Author: Lars Krogh
// Version: 1.0.0
func (c *ChangeItem) ID() string { return c.Change.Target.ID() }

// Plan holds one item per selected optimization, in catalog order.
type Plan struct {
	Items []*Item
}

// Build selects optimizations, checks their applicability to the release,
// resolves power settings to the scheme that will be active, and applies the
// protection rules. Items that pass are pending until Evaluate compares them
// with the inspected state.
//
// Author: Lars Krogh
// Version: 1.0.0
func Build(all []catalog.Optimization, opts Options, release facts.Release, f *facts.Facts) *Plan {
	p := &Plan{}
	for _, o := range all {
		selectedBy := selection(o, opts)
		if selectedBy == "" {
			continue
		}
		item := &Item{Optimization: o, SelectedBy: selectedBy}
		p.Items = append(p.Items, item)
		for _, c := range o.Changes {
			item.Changes = append(item.Changes, &ChangeItem{OptimizationID: o.ID, RebootRequired: o.RebootRequired, Change: c, Status: ChangePending})
		}
		if pattern, ok := matchAny(opts.Exclude, o.ID); ok {
			item.skip("excluded by configuration (" + pattern + ")")
			continue
		}
		if ok, why := o.Applies.Check(release); !ok {
			item.Status, item.Reason = StatusUnsupported, why
			item.setChanges(ChangeNotApplicable, why)
			continue
		}
		item.Status = StatusPending
	}
	scheme := p.targetScheme(f)
	for _, item := range p.Items {
		if item.Status == StatusPending {
			item.resolve(scheme, f)
		}
	}
	return p
}

// selection decides whether opts select o.
//
// Returns:
//   - string: How the optimization was selected, or "" when it was not.
//
// Author: Lars Krogh
// Version: 1.0.0
func selection(o catalog.Optimization, opts Options) string {
	for _, pattern := range opts.Include {
		exact := strings.EqualFold(pattern, o.ID)
		if exact || (o.Classification != catalog.ClassHighRisk && MatchPattern(pattern, o.ID)) {
			return "include " + pattern
		}
	}
	if !opts.Profile.Includes(o.Profile) {
		return ""
	}
	if len(opts.Categories) > 0 && !slices.Contains(opts.Categories, o.Category) {
		return ""
	}
	return "profile " + string(opts.Profile)
}

// targetScheme returns the power scheme that will be active after the plan is
// applied: the scheme a pending power_scheme change activates when it is
// installed, otherwise the active scheme. It returns "" when the power
// configuration is unknown.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) targetScheme(f *facts.Facts) string {
	if f == nil || f.Power == nil {
		return ""
	}
	for _, item := range p.Items {
		if item.Status != StatusPending {
			continue
		}
		for _, c := range item.Changes {
			if c.Change.Target.Kind == catalog.KindPowerScheme && f.Power.HasScheme(c.Change.Want.Data) {
				return strings.ToLower(c.Change.Want.Data)
			}
		}
	}
	return f.Power.ActiveScheme
}

// resolve fills in the power scheme of power settings, marks power scheme
// changes whose scheme is not installed as not applicable, and skips the whole
// item when a protection rule objects to any of its changes. An optimization is
// applied completely or not at all.
//
// Author: Lars Krogh
// Version: 1.0.0
func (it *Item) resolve(scheme string, f *facts.Facts) {
	for _, c := range it.Changes {
		t := &c.Change.Target
		switch t.Kind {
		case catalog.KindPowerSetting:
			if scheme == "" {
				it.skip("the power configuration could not be detected")
				return
			}
			t.Scheme = scheme
		case catalog.KindPowerScheme:
			if f == nil || f.Power == nil {
				it.skip("the power configuration could not be detected")
				return
			}
			if !f.Power.HasScheme(c.Change.Want.Data) {
				c.Status, c.Reason = ChangeNotApplicable, "the power scheme "+c.Change.Want.Data+" is not installed"
			}
		}
		if why := Protect(c.Change, f); why != "" {
			it.skip(why)
			c.Reason = why
			return
		}
	}
	if it.countChanges(ChangeNotApplicable) == len(it.Changes) {
		it.Status, it.Reason = StatusUnsupported, it.Changes[0].Reason
	}
}

// skip marks the item and every change in it as skipped.
//
// Author: Lars Krogh
// Version: 1.0.0
func (it *Item) skip(why string) {
	it.Status, it.Reason = StatusSkipped, why
	it.setChanges(ChangeSkipped, "the optimization was skipped")
}

// setChanges sets the status and reason of every change in the item.
//
// Author: Lars Krogh
// Version: 1.0.0
func (it *Item) setChanges(s ChangeStatus, why string) {
	for _, c := range it.Changes {
		c.Status, c.Reason = s, why
	}
}

// countChanges returns the number of changes in the item with status s.
//
// Author: Lars Krogh
// Version: 1.0.0
func (it *Item) countChanges(s ChangeStatus) int {
	n := 0
	for _, c := range it.Changes {
		if c.Status == s {
			n++
		}
	}
	return n
}

// Evaluate compares every pending change with the inspected state and decides
// each pending item: failed when a target could not be inspected, skipped when
// a value to be changed could not be restored, unsupported when none of its
// targets exist, pending when something must change, and compliant otherwise.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) Evaluate(states States) {
	for _, it := range p.Items {
		if it.Status != StatusPending {
			continue
		}
		for _, c := range it.Changes {
			if c.Status == ChangePending {
				evaluateChange(c, states)
			}
		}
		switch {
		case it.countChanges(ChangeFailed) > 0:
			it.Status, it.Reason = StatusFailed, "a target could not be inspected"
		case it.countChanges(ChangeSkipped) > 0:
			why := firstReason(it.Changes, ChangeSkipped)
			for _, c := range it.Changes {
				if c.Status == ChangePending {
					c.Status, c.Reason = ChangeSkipped, "the optimization was skipped"
				}
			}
			it.Status, it.Reason = StatusSkipped, why
		case it.countChanges(ChangeNotApplicable) == len(it.Changes):
			it.Status, it.Reason = StatusUnsupported, "none of its targets exist on this system"
		case it.countChanges(ChangePending) > 0:
			it.Status = StatusPending
		default:
			it.Status = StatusCompliant
		}
	}
}

// evaluateChange compares one change with its inspected state.
//
// Author: Lars Krogh
// Version: 1.0.0
func evaluateChange(c *ChangeItem, states States) {
	st, ok := states.Get(c.ID())
	if !ok {
		c.Status, c.Reason = ChangeFailed, "the guest returned no inspection result"
		return
	}
	c.Before = &st
	if st.Error != nil {
		c.Status, c.Reason = ChangeFailed, "inspection failed: "+*st.Error
		return
	}
	switch Compare(c.Change, st) {
	case Match:
		c.Status = ChangeCompliant
	case NotApplicable:
		c.Status, c.Reason = ChangeNotApplicable, "the target does not exist on this system"
	default:
		restore, err := restoreValue(c.Change.Target.Kind, st)
		if err != nil {
			c.Status, c.Reason = ChangeSkipped, "not changed because the current value could not be restored: "+err.Error()
			return
		}
		c.Restore = &restore
	}
}

// firstReason returns the reason of the first change with status s.
//
// Author: Lars Krogh
// Version: 1.0.0
func firstReason(changes []*ChangeItem, s ChangeStatus) string {
	for _, c := range changes {
		if c.Status == s {
			return c.Reason
		}
	}
	return ""
}

// Pending returns the pending changes of pending items: before Evaluate the
// targets to inspect, after Evaluate the changes to apply.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) Pending() []*ChangeItem {
	var out []*ChangeItem
	for _, it := range p.Items {
		if it.Status != StatusPending {
			continue
		}
		for _, c := range it.Changes {
			if c.Status == ChangePending {
				out = append(out, c)
			}
		}
	}
	return out
}

// Verify sets the outcome of applied changes. A change is applied only when the
// guest reported success and the re-inspected value matches. An item is
// applied when every one of its pending changes is applied, and failed
// otherwise.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) Verify(results ApplyResults, after States) {
	for _, it := range p.Items {
		if it.Status != StatusPending {
			continue
		}
		failed := 0
		for _, c := range it.Changes {
			if c.Status != ChangePending {
				continue
			}
			verifyChange(c, results, after, ChangeApplied)
			if c.Status == ChangeFailed {
				failed++
			}
		}
		if failed > 0 {
			it.Status, it.Reason = StatusFailed, fmt.Sprintf("%d changes failed to apply or verify", failed)
			continue
		}
		it.Status = StatusApplied
	}
}

// verifyChange sets c to success when the guest applied it and the state after
// matches, and to failed otherwise.
//
// Author: Lars Krogh
// Version: 1.0.0
func verifyChange(c *ChangeItem, results ApplyResults, after States, success ChangeStatus) {
	res, ok := results.Get(c.ID())
	st, inspected := after.Get(c.ID())
	if inspected {
		c.After = &st
	}
	switch {
	case !ok:
		c.Status, c.Reason = ChangeFailed, "the guest returned no apply result"
	case !res.OK:
		c.Status, c.Reason = ChangeFailed, "apply failed: "+deref(res.Error)
	case !inspected:
		c.Status, c.Reason = ChangeFailed, "applied but could not be re-inspected"
	case st.Error != nil:
		c.Status, c.Reason = ChangeFailed, "verification failed: "+*st.Error
	case Compare(c.Change, st) != Match:
		c.Status, c.Reason = ChangeFailed, "verification failed: the value after applying is "+st.String()
	default:
		c.Status = success
	}
}

// Verified returns the applied and compliant changes, which are re-checked
// after a restart.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) Verified() []*ChangeItem {
	var out []*ChangeItem
	for _, it := range p.Items {
		if it.Status != StatusApplied && it.Status != StatusCompliant {
			continue
		}
		for _, c := range it.Changes {
			if c.Status == ChangeApplied || c.Status == ChangeCompliant {
				out = append(out, c)
			}
		}
	}
	return out
}

// Reverify re-checks applied and compliant items against current and marks
// those that no longer match as failed. when is appended to the reason, for
// example "after restart".
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) Reverify(current States, when string) {
	for _, it := range p.Items {
		if it.Status != StatusApplied && it.Status != StatusCompliant {
			continue
		}
		for _, c := range it.Changes {
			if c.Status != ChangeApplied && c.Status != ChangeCompliant {
				continue
			}
			st, ok := current.Get(c.ID())
			switch {
			case !ok:
				c.Status, c.Reason = ChangeFailed, "could not be re-inspected "+when
			case st.Error != nil:
				c.Status, c.Reason = ChangeFailed, "re-inspection "+when+" failed: "+*st.Error
			case Compare(c.Change, st) != Match:
				c.After = &st
				c.Status, c.Reason = ChangeFailed, "no longer in the wanted state "+when+": the value is "+st.String()
			default:
				continue
			}
			it.Status, it.Reason = StatusFailed, "a change did not survive the restart"
		}
	}
}

// Count returns the number of items with status s.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) Count(s Status) int {
	n := 0
	for _, it := range p.Items {
		if it.Status == s {
			n++
		}
	}
	return n
}

// RebootRequired reports whether an applied optimization needs a restart to
// take full effect.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) RebootRequired() bool {
	for _, it := range p.Items {
		if it.Status == StatusApplied && it.Optimization.RebootRequired {
			return true
		}
	}
	return false
}

// matchAny returns the first pattern that matches id.
//
// Returns:
//   - string: The matching pattern.
//   - bool: Whether any pattern matched.
//
// Author: Lars Krogh
// Version: 1.0.0
func matchAny(patterns []string, id string) (string, bool) {
	for _, p := range patterns {
		if MatchPattern(p, id) {
			return p, true
		}
	}
	return "", false
}

// MatchPattern reports whether an optimization ID matches a pattern. The match
// is case-insensitive and covers the whole ID, and '*' matches any sequence.
//
// Author: Lars Krogh
// Version: 1.0.0
func MatchPattern(pattern, id string) bool {
	pattern, id = strings.ToLower(pattern), strings.ToLower(id)
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == id
	}
	if !strings.HasPrefix(id, parts[0]) {
		return false
	}
	id = id[len(parts[0]):]
	for _, mid := range parts[1 : len(parts)-1] {
		i := strings.Index(id, mid)
		if i < 0 {
			return false
		}
		id = id[i+len(mid):]
	}
	return strings.HasSuffix(id, parts[len(parts)-1])
}
