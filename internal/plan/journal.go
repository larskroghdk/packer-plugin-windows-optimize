package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/catalog"
)

const JournalSchemaVersion = 1

// Journal is the rollback journal kept on the guest. It holds the original
// value of every target the plugin has changed, recorded before the first
// change, so a later run can restore it.
type Journal struct {
	SchemaVersion int            `json:"schema_version"`
	Entries       []JournalEntry `json:"entries"`
}

// JournalEntry is the original value of one target.
type JournalEntry struct {
	ID             string         `json:"id"`
	Optimization   string         `json:"optimization"`
	Target         catalog.Target `json:"target"`
	Original       catalog.Value  `json:"original"`
	RebootRequired bool           `json:"reboot_required"`
	RecordedAt     time.Time      `json:"recorded_at"`
}

// ParseJournal decodes the journal the guest returned. Empty data means there
// is no journal yet.
//
// Returns:
//   - *Journal: The journal, empty when there is none.
//   - error: The data is not valid JSON or has an unknown schema version.
//
// Author: Lars Krogh
// Version: 1.0.0
func ParseJournal(data []byte) (*Journal, error) {
	j := &Journal{SchemaVersion: JournalSchemaVersion, Entries: []JournalEntry{}}
	if len(bytes.TrimSpace(data)) == 0 {
		return j, nil
	}
	if err := json.Unmarshal(data, j); err != nil {
		return nil, fmt.Errorf("decode rollback journal: %w", err)
	}
	if j.SchemaVersion != JournalSchemaVersion {
		return nil, fmt.Errorf("rollback journal has schema version %d, this plugin reads version %d", j.SchemaVersion, JournalSchemaVersion)
	}
	if j.Entries == nil {
		j.Entries = []JournalEntry{}
	}
	return j, nil
}

// Record adds the original value of each change that is about to be applied.
// A target that is already in the journal keeps its entry, so the journal
// always holds the value from before the first run that changed it.
//
// Returns:
//   - int: The number of entries added.
//
// Author: Lars Krogh
// Version: 1.0.0
func (j *Journal) Record(changes []*ChangeItem, now time.Time) int {
	known := map[string]bool{}
	for _, e := range j.Entries {
		known[strings.ToLower(e.ID)] = true
	}
	added := 0
	for _, c := range changes {
		if c.Restore == nil || known[strings.ToLower(c.ID())] {
			continue
		}
		j.Entries = append(j.Entries, JournalEntry{
			ID:             c.ID(),
			Optimization:   c.OptimizationID,
			Target:         c.Change.Target,
			Original:       *c.Restore,
			RebootRequired: c.RebootRequired,
			RecordedAt:     now.UTC(),
		})
		known[strings.ToLower(c.ID())] = true
		added++
	}
	return added
}

// Rollback is the plan for restoring the journal: one change per entry, which
// sets the target back to its original value.
type Rollback struct {
	Changes []*ChangeItem
}

// NewRollback creates a pending rollback change for every journal entry. An
// entry whose target is not set by any catalog optimization, or whose ID does
// not match its target, fails without being inspected or applied, so an edited
// journal cannot make rollback change a target the catalog never changes, such
// as a WinRM dependency.
//
// Parameters:
//   - all: The optimization catalog.
//
// Author: Lars Krogh
// Version: 1.1.0
func NewRollback(j *Journal, all []catalog.Optimization) *Rollback {
	known := map[string]bool{}
	for _, o := range all {
		for _, c := range o.Changes {
			known[strings.ToLower(c.Target.ID())] = true
		}
	}
	r := &Rollback{}
	for _, e := range j.Entries {
		c := &ChangeItem{
			OptimizationID: e.Optimization,
			RebootRequired: e.RebootRequired,
			Change:         catalog.Change{Target: e.Target, Want: e.Original},
			Status:         ChangePending,
		}
		generic := e.Target
		generic.Scheme = ""
		switch {
		case !strings.EqualFold(e.ID, e.Target.ID()):
			c.Status, c.Reason = ChangeFailed, "the journal entry ID does not match its target; it was not restored"
		case !known[strings.ToLower(generic.ID())]:
			c.Status, c.Reason = ChangeFailed, "no catalog optimization changes this target; it was not restored"
		}
		r.Changes = append(r.Changes, c)
	}
	return r
}

// Evaluate compares each pending rollback change with the inspected state. A
// target already at its original value is compliant, a target that no longer
// exists is not applicable, and anything else stays pending.
//
// Author: Lars Krogh
// Version: 1.1.0
func (r *Rollback) Evaluate(states States) {
	for _, c := range r.Pending() {
		st, ok := states.Get(c.ID())
		switch {
		case !ok:
			c.Status, c.Reason = ChangeFailed, "the guest returned no inspection result"
			continue
		case st.Error != nil:
			c.Status, c.Reason = ChangeFailed, "inspection failed: "+*st.Error
			continue
		}
		c.Before = &st
		switch Compare(c.Change, st) {
		case Match:
			c.Status, c.Reason = ChangeCompliant, "already at the original value"
		case NotApplicable:
			c.Status, c.Reason = ChangeNotApplicable, "the target no longer exists"
		}
	}
}

// Pending returns the rollback changes that still have to be applied.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *Rollback) Pending() []*ChangeItem {
	var out []*ChangeItem
	for _, c := range r.Changes {
		if c.Status == ChangePending {
			out = append(out, c)
		}
	}
	return out
}

// Verify sets each pending rollback change to restored when the guest applied
// it and the re-inspected value is the original one, and to failed otherwise.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *Rollback) Verify(results ApplyResults, after States) {
	for _, c := range r.Pending() {
		verifyChange(c, results, after, ChangeRestored)
	}
}

// Restored returns the rollback changes that were restored.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *Rollback) Restored() []*ChangeItem {
	var out []*ChangeItem
	for _, c := range r.Changes {
		if c.Status == ChangeRestored {
			out = append(out, c)
		}
	}
	return out
}

// Reverify marks restored changes whose target no longer holds the original
// value as failed. when is appended to the reason, for example "after
// restart".
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *Rollback) Reverify(current States, when string) {
	for _, c := range r.Restored() {
		st, ok := current.Get(c.ID())
		switch {
		case !ok:
			c.Status, c.Reason = ChangeFailed, "could not be re-inspected "+when
		case st.Error != nil:
			c.Status, c.Reason = ChangeFailed, "re-inspection "+when+" failed: "+*st.Error
		case Compare(c.Change, st) != Match:
			c.After = &st
			c.Status, c.Reason = ChangeFailed, "no longer at the original value "+when+": the value is "+st.String()
		}
	}
}

// Remaining returns the journal entries that still need restoring: those whose
// rollback change failed or was not run.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *Rollback) Remaining(j *Journal) *Journal {
	done := map[string]bool{}
	for _, c := range r.Changes {
		switch c.Status {
		case ChangeRestored, ChangeCompliant, ChangeNotApplicable:
			done[strings.ToLower(c.ID())] = true
		}
	}
	out := &Journal{SchemaVersion: JournalSchemaVersion, Entries: []JournalEntry{}}
	for _, e := range j.Entries {
		if !done[strings.ToLower(e.ID)] {
			out.Entries = append(out.Entries, e)
		}
	}
	return out
}

// Count returns the number of rollback changes with status s.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *Rollback) Count(s ChangeStatus) int {
	n := 0
	for _, c := range r.Changes {
		if c.Status == s {
			n++
		}
	}
	return n
}

// RebootRequired reports whether a restored change needs a restart to take
// full effect.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *Rollback) RebootRequired() bool {
	for _, c := range r.Changes {
		if c.Status == ChangeRestored && c.RebootRequired {
			return true
		}
	}
	return false
}
