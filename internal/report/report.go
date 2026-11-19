// Package report builds the JSON optimization report and the console summary.
//
// Increment SchemaVersion for any incompatible change to the JSON.
package report

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/facts"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/plan"
)

const SchemaVersion = 1

const (
	ModeApply          = "apply"
	ModeDryRun         = "dry_run"
	ModeRollback       = "rollback"
	ModeRollbackDryRun = "rollback_dry_run"
)

// Report is the JSON document written at the end of a build.
type Report struct {
	SchemaVersion int            `json:"schema_version"`
	Plugin        string         `json:"plugin"`
	StartedAt     time.Time      `json:"started_at"`
	FinishedAt    time.Time      `json:"finished_at"`
	Mode          string         `json:"mode"`
	Profile       string         `json:"profile"`
	System        System         `json:"system"`
	Summary       Summary        `json:"summary"`
	Rollback      *RollbackInfo  `json:"rollback,omitempty"`
	Journal       Journal        `json:"journal"`
	Connectivity  []Check        `json:"connectivity"`
	Reboot        Reboot         `json:"reboot"`
	Warnings      []string       `json:"warnings"`
	Optimizations []Optimization `json:"optimizations"`
}

// System describes the guest as detected.
type System struct {
	ProductName     string   `json:"product_name"`
	Release         string   `json:"release"`
	KnownRelease    bool     `json:"known_release"`
	Installation    string   `json:"installation"`
	EditionID       string   `json:"edition_id"`
	Version         string   `json:"version"`
	DisplayVersion  string   `json:"display_version"`
	Architecture    string   `json:"architecture"`
	WinRMUser       string   `json:"winrm_user"`
	ActivePower     string   `json:"active_power_scheme"`
	DetectionErrors []string `json:"detection_errors"`
}

// Summary counts the selected optimizations by status.
type Summary struct {
	Selected    int `json:"selected"`
	Applied     int `json:"applied"`
	Compliant   int `json:"compliant"`
	Pending     int `json:"pending"`
	Skipped     int `json:"skipped"`
	Unsupported int `json:"unsupported"`
	Failed      int `json:"failed"`
}

// RollbackInfo is the outcome of a rollback run, one change per journal entry.
type RollbackInfo struct {
	Entries       int      `json:"entries"`
	Restored      int      `json:"restored"`
	Compliant     int      `json:"compliant"`
	NotApplicable int      `json:"not_applicable"`
	Pending       int      `json:"pending"`
	Failed        int      `json:"failed"`
	Changes       []Change `json:"changes"`
}

// Journal describes the rollback journal on the guest after the run.
type Journal struct {
	Path    string `json:"path"`
	Entries int    `json:"entries"`
	Added   int    `json:"added"`
}

// Check is one WinRM connectivity check made during the build.
type Check struct {
	Name    string    `json:"name"`
	At      time.Time `json:"at"`
	Passed  bool      `json:"passed"`
	Details string    `json:"details"`
}

// Reboot records whether a restart is needed and whether it was performed.
type Reboot struct {
	Required  bool   `json:"required"`
	Performed bool   `json:"performed"`
	Reason    string `json:"reason,omitempty"`
}

// Optimization is the report entry for one selected optimization.
type Optimization struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Classification string   `json:"classification"`
	SelectedBy     string   `json:"selected_by"`
	Status         string   `json:"status"`
	Reason         string   `json:"reason,omitempty"`
	RebootRequired bool     `json:"reboot_required"`
	References     []string `json:"references"`
	Changes        []Change `json:"changes"`
}

// Change is the report entry for one change.
type Change struct {
	ID           string `json:"id"`
	Optimization string `json:"optimization,omitempty"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	Expected     string `json:"expected"`
	Before       string `json:"before,omitempty"`
	After        string `json:"after,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// Input is everything Build needs. Facts, Release, Plan and Rollback are nil
// when the run failed before they were known or did not use them.
type Input struct {
	Plugin       string
	StartedAt    time.Time
	FinishedAt   time.Time
	Mode         string
	Profile      string
	Facts        *facts.Facts
	Release      *facts.Release
	Plan         *plan.Plan
	Rollback     *plan.Rollback
	Journal      Journal
	Connectivity []Check
	Reboot       Reboot
	Warnings     []string
}

// Build assembles the report from what the run produced, including when the
// run failed early.
//
// Author: Lars Krogh
// Version: 1.0.0
func Build(in Input) *Report {
	r := &Report{
		SchemaVersion: SchemaVersion,
		Plugin:        in.Plugin,
		StartedAt:     in.StartedAt.UTC(),
		FinishedAt:    in.FinishedAt.UTC(),
		Mode:          in.Mode,
		Profile:       in.Profile,
		Journal:       in.Journal,
		Connectivity:  nonNil(in.Connectivity),
		Reboot:        in.Reboot,
		Warnings:      nonNil(in.Warnings),
		Optimizations: []Optimization{},
	}
	if in.Facts != nil {
		r.System = system(in.Facts, in.Release)
	}
	if in.Plan != nil {
		for _, it := range in.Plan.Items {
			r.Optimizations = append(r.Optimizations, optimization(it))
		}
		r.Summary = Summary{
			Selected:    len(in.Plan.Items),
			Applied:     in.Plan.Count(plan.StatusApplied),
			Compliant:   in.Plan.Count(plan.StatusCompliant),
			Pending:     in.Plan.Count(plan.StatusPending),
			Skipped:     in.Plan.Count(plan.StatusSkipped),
			Unsupported: in.Plan.Count(plan.StatusUnsupported),
			Failed:      in.Plan.Count(plan.StatusFailed),
		}
	}
	if in.Rollback != nil {
		info := &RollbackInfo{
			Entries:       len(in.Rollback.Changes),
			Restored:      in.Rollback.Count(plan.ChangeRestored),
			Compliant:     in.Rollback.Count(plan.ChangeCompliant),
			NotApplicable: in.Rollback.Count(plan.ChangeNotApplicable),
			Pending:       in.Rollback.Count(plan.ChangePending),
			Failed:        in.Rollback.Count(plan.ChangeFailed),
			Changes:       []Change{},
		}
		for _, c := range in.Rollback.Changes {
			info.Changes = append(info.Changes, change(c, true))
		}
		r.Rollback = info
	}
	return r
}

// nonNil returns s, or an empty slice when s is nil, so the JSON has [] rather
// than null.
//
// Author: Lars Krogh
// Version: 1.0.0
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// system converts the guest facts and release to the report's system section.
//
// Author: Lars Krogh
// Version: 1.0.0
func system(f *facts.Facts, rel *facts.Release) System {
	s := System{
		ProductName:     f.OS.ProductName,
		EditionID:       f.OS.EditionID,
		Version:         f.OS.Version(),
		DisplayVersion:  f.OS.DisplayVersion,
		Architecture:    f.OS.Architecture,
		WinRMUser:       f.Session.UserName,
		DetectionErrors: nonNil(f.Errors),
	}
	if rel != nil {
		s.Release, s.KnownRelease, s.Installation = rel.Name, rel.Known, string(rel.Installation)
	}
	if f.Power != nil {
		s.ActivePower = f.Power.ActiveScheme
	}
	return s
}

// optimization converts a plan item to its report entry.
//
// Author: Lars Krogh
// Version: 1.0.0
func optimization(it *plan.Item) Optimization {
	o := it.Optimization
	out := Optimization{
		ID:             o.ID,
		Name:           o.Name,
		Category:       string(o.Category),
		Classification: string(o.Classification),
		SelectedBy:     it.SelectedBy,
		Status:         string(it.Status),
		Reason:         it.Reason,
		RebootRequired: o.RebootRequired,
		References:     []string{},
		Changes:        []Change{},
	}
	for _, ref := range o.References {
		out.References = append(out.References, ref.URL)
	}
	for _, c := range it.Changes {
		out.Changes = append(out.Changes, change(c, false))
	}
	return out
}

// change converts a change item to its report entry. withOptimization adds the
// optimization ID, which the rollback section needs because it is flat.
//
// Author: Lars Krogh
// Version: 1.0.0
func change(c *plan.ChangeItem, withOptimization bool) Change {
	out := Change{
		ID:       c.ID(),
		Kind:     string(c.Change.Target.Kind),
		Status:   string(c.Status),
		Expected: c.Change.Want.String(),
		Reason:   c.Reason,
	}
	if withOptimization {
		out.Optimization = c.OptimizationID
	}
	if c.Before != nil {
		out.Before = c.Before.String()
	}
	if c.After != nil {
		out.After = c.After.String()
	}
	return out
}

// JSON returns the report as indented JSON with a trailing newline.
//
// Returns:
//   - []byte: The encoded report.
//   - error: The report cannot be encoded.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *Report) JSON() ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// SummaryLines returns the console summary: system, mode and profile, counts,
// reboot state, then every optimization that is pending, skipped or failed,
// and every warning.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *Report) SummaryLines() []string {
	lines := []string{
		fmt.Sprintf("System:     %s (%s, %s, build %s)", r.System.Release, r.System.EditionID, r.System.Installation, r.System.Version),
		fmt.Sprintf("Mode:       %s, profile %s", r.Mode, r.Profile),
	}
	if r.Rollback != nil {
		rb := r.Rollback
		lines = append(lines, fmt.Sprintf("Rollback:   %d journal entries: %d restored, %d already original, %d not applicable, %d pending, %d failed",
			rb.Entries, rb.Restored, rb.Compliant, rb.NotApplicable, rb.Pending, rb.Failed))
		for _, c := range rb.Changes {
			if c.Status == string(plan.ChangeFailed) || c.Status == string(plan.ChangePending) {
				lines = append(lines, fmt.Sprintf("  %-9s %s (%s): %s", c.Status, c.ID, c.Optimization, c.Reason))
			}
		}
	} else {
		n := r.Summary
		lines = append(lines, fmt.Sprintf("Optimized:  %d selected: %d applied, %d already compliant, %d pending, %d skipped, %d unsupported, %d failed",
			n.Selected, n.Applied, n.Compliant, n.Pending, n.Skipped, n.Unsupported, n.Failed))
		for _, o := range r.Optimizations {
			switch o.Status {
			case string(plan.StatusPending), string(plan.StatusSkipped), string(plan.StatusFailed):
				lines = append(lines, fmt.Sprintf("  %-9s %s: %s", o.Status, o.ID, describe(o)))
			}
		}
	}
	if r.Reboot.Required {
		state := "required, not performed; add a windows-restart provisioner or set reboot_if_required"
		if r.Reboot.Performed {
			state = "performed"
		}
		lines = append(lines, fmt.Sprintf("Reboot:     %s (%s)", state, r.Reboot.Reason))
	}
	for _, w := range r.Warnings {
		lines = append(lines, "Warning:    "+w)
	}
	return lines
}

// describe returns the reason shown for an optimization in the summary, or its
// name when there is no reason.
//
// Author: Lars Krogh
// Version: 1.0.0
func describe(o Optimization) string {
	if o.Reason != "" {
		return o.Reason
	}
	return o.Name
}
