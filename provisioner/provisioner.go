// Package provisioner implements the windows-optimize Packer provisioner: it
// uploads the guest scripts, detects the OS, plans the selected optimizations
// under the WinRM protection rules, records the rollback journal, applies and
// verifies the changes, checks WinRM, and writes the report.
package provisioner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/uuid"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/catalog"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/facts"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/plan"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/report"
)

var Version = "dev"

// Provisioner is the windows-optimize provisioner registered with Packer.
type Provisioner struct {
	config  Config
	catalog []catalog.Optimization
	now     func() time.Time
}

// ConfigSpec returns the HCL2 specification of Config.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Provisioner) ConfigSpec() hcldec.ObjectSpec { return p.config.FlatMapstructure().HCL2Spec() }

// Prepare validates the embedded catalog and the configuration.
//
// Returns:
//   - error: The catalog is inconsistent or the configuration is invalid.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Provisioner) Prepare(raws ...any) error {
	all := catalog.All()
	if err := catalog.Validate(all); err != nil {
		return fmt.Errorf("embedded optimization catalog is invalid: %w", err)
	}
	p.catalog = all
	if p.now == nil {
		p.now = time.Now
	}
	return p.config.prepare(all, raws...)
}

// Provision optimizes, or rolls back, the guest over comm. The report is
// written whether or not the run succeeds, and the guest working directory is
// removed.
//
// Parameters:
//   - generatedData: Builder data. ConnType must be winrm when it is set.
//
// Returns:
//   - error: The run failed, or the report could not be written.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Provisioner) Provision(ctx context.Context, ui packer.Ui, comm packer.Communicator, generatedData map[string]any) error {
	if connType, _ := generatedData["ConnType"].(string); connType != "" && connType != "winrm" {
		return fmt.Errorf("windows-optimize requires the WinRM communicator, the build uses %q", connType)
	}
	r := &run{
		cfg:     &p.config,
		catalog: p.catalog,
		ui:      ui,
		remote:  newRemote(comm, ui, uuid.TimeOrderedUUID()),
		now:     p.now,
		started: p.now(),
	}
	var runErr error
	if p.config.Rollback {
		runErr = r.rollback(ctx)
	} else {
		runErr = r.optimize(ctx)
	}
	if err := r.remote.cleanup(ctx); err != nil {
		r.warn("could not remove the guest working directory %s: %v", r.remote.dir, err)
	}
	result := r.report()
	for _, line := range result.SummaryLines() {
		ui.Say(line)
	}
	if err := writeReport(p.config.ReportPath, result); err != nil {
		ui.Error(fmt.Sprintf("Could not write the optimization report: %v", err))
		if runErr == nil {
			runErr = err
		}
	} else {
		ui.Say("Report:     " + p.config.ReportPath)
	}
	return runErr
}

// writeReport writes the report as JSON to path, creating missing
// directories.
//
// Returns:
//   - error: The report cannot be encoded or written.
//
// Author: Lars Krogh
// Version: 1.0.0
func writeReport(path string, result *report.Report) error {
	data, err := result.JSON()
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

type run struct {
	cfg      *Config
	catalog  []catalog.Optimization
	ui       packer.Ui
	remote   *remote
	now      func() time.Time
	started  time.Time
	facts    *facts.Facts
	release  *facts.Release
	plan     *plan.Plan
	undo     *plan.Rollback
	journal  report.Journal
	checks   []report.Check
	reboot   report.Reboot
	warnings []string
}

type targetRequest struct {
	ID     string         `json:"id"`
	Target catalog.Target `json:"target"`
}

type applyRequest struct {
	ID     string         `json:"id"`
	Target catalog.Target `json:"target"`
	Want   catalog.Value  `json:"want"`
}

// warn records a warning for the report and shows it in the Packer UI.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	r.warnings = append(r.warnings, msg)
	r.ui.Error("Warning: " + msg)
}

// mode returns the report mode of the run.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) mode() string {
	switch {
	case r.cfg.Rollback && r.cfg.DryRun:
		return report.ModeRollbackDryRun
	case r.cfg.Rollback:
		return report.ModeRollback
	case r.cfg.DryRun:
		return report.ModeDryRun
	}
	return report.ModeApply
}

// report builds the report from the run's current state.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) report() *report.Report {
	return report.Build(report.Input{
		Plugin:       "packer-plugin-windows-optimize " + Version,
		StartedAt:    r.started,
		FinishedAt:   r.now(),
		Mode:         r.mode(),
		Profile:      r.cfg.Profile,
		Facts:        r.facts,
		Release:      r.release,
		Plan:         r.plan,
		Rollback:     r.undo,
		Journal:      r.journal,
		Connectivity: r.checks,
		Reboot:       r.reboot,
		Warnings:     r.warnings,
	})
}

// optimize runs the optimization steps in order: upload, detect, plan,
// inspect, record the rollback journal, apply, verify, check WinRM, and
// optionally restart and re-verify. In dry-run mode it stops after inspection.
//
// Returns:
//   - error: A step failed, WinRM regressed, or optimizations failed while
//     fail_on_error is true.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) optimize(ctx context.Context) error {
	if err := r.prepareGuest(ctx); err != nil {
		return err
	}
	r.plan = plan.Build(r.catalog, r.cfg.options(), *r.release, r.facts)
	targets := r.plan.Pending()
	r.ui.Say(fmt.Sprintf("Inspecting %d targets of %d selected optimizations (profile %s)...", len(targets), len(r.plan.Items), r.cfg.Profile))
	states, err := r.inspect(ctx, targets)
	if err != nil {
		return err
	}
	r.plan.Evaluate(states)
	pending := r.plan.Pending()
	r.ui.Say(fmt.Sprintf("Plan: %d already compliant, %d to apply (%d changes), %d skipped, %d unsupported, %d failed inspection",
		r.plan.Count(plan.StatusCompliant), r.plan.Count(plan.StatusPending), len(pending),
		r.plan.Count(plan.StatusSkipped), r.plan.Count(plan.StatusUnsupported), r.plan.Count(plan.StatusFailed)))
	if r.cfg.DryRun {
		r.ui.Say("Dry run: no changes were made.")
		return nil
	}
	if len(pending) > 0 {
		if err := r.recordJournal(ctx, pending); err != nil {
			return err
		}
		r.ui.Say(fmt.Sprintf("Applying %d changes...", len(pending)))
		results, after, err := r.applyAndInspect(ctx, pending)
		if err != nil {
			return err
		}
		r.plan.Verify(results, after)
		if err := r.checkConnectivity(ctx, "after applying optimizations"); err != nil {
			return err
		}
	}
	if r.plan.RebootRequired() {
		r.reboot.Required = true
		r.reboot.Reason = "applied optimizations take full effect only after a restart"
		if r.cfg.RebootIfRequired {
			if err := r.restartAndReverify(ctx, r.plan.Verified(), r.plan.Reverify); err != nil {
				return err
			}
		}
	}
	if failed := r.plan.Count(plan.StatusFailed); failed > 0 {
		return r.failure("%d optimizations failed to apply or verify (see the report)", failed)
	}
	return nil
}

// rollback restores every target in the rollback journal to its original
// value, verifies it, and keeps only the entries that could not be restored.
// In dry-run mode it stops after inspection.
//
// Returns:
//   - error: A step failed, WinRM regressed, or entries failed to restore
//     while fail_on_error is true.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) rollback(ctx context.Context) error {
	if err := r.prepareGuest(ctx); err != nil {
		return err
	}
	journal, err := r.readJournal(ctx)
	if err != nil {
		return err
	}
	r.undo = plan.NewRollback(journal, r.catalog)
	r.journal.Entries = len(journal.Entries)
	if len(journal.Entries) == 0 {
		r.ui.Say("The rollback journal is empty: nothing to restore.")
		return nil
	}
	r.ui.Say(fmt.Sprintf("Inspecting %d journal entries...", len(journal.Entries)))
	states, err := r.inspect(ctx, r.undo.Pending())
	if err != nil {
		return err
	}
	r.undo.Evaluate(states)
	pending := r.undo.Pending()
	if r.cfg.DryRun {
		r.ui.Say(fmt.Sprintf("Dry run: %d changes would be restored; no changes were made.", len(pending)))
		return nil
	}
	if len(pending) > 0 {
		r.ui.Say(fmt.Sprintf("Restoring %d original values...", len(pending)))
		results, after, err := r.applyAndInspect(ctx, pending)
		if err != nil {
			return err
		}
		r.undo.Verify(results, after)
	}
	if err := r.writeJournal(ctx, r.undo.Remaining(journal)); err != nil {
		return err
	}
	if len(pending) > 0 {
		if err := r.checkConnectivity(ctx, "after rollback"); err != nil {
			return err
		}
	}
	if r.undo.RebootRequired() {
		r.reboot.Required = true
		r.reboot.Reason = "restored values take full effect only after a restart"
		if r.cfg.RebootIfRequired {
			if err := r.restartAndReverify(ctx, r.undo.Restored(), r.undo.Reverify); err != nil {
				return err
			}
		}
	}
	if failed := r.undo.Count(plan.ChangeFailed); failed > 0 {
		return r.failure("%d journal entries could not be restored and were kept in the journal (see the report)", failed)
	}
	return nil
}

// failure returns an error built from format, or records it as a warning and
// returns nil when fail_on_error is false.
//
// Returns:
//   - error: The failure, or nil when failures do not fail the build.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) failure(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if r.cfg.failOnError() {
		return errors.New(msg)
	}
	r.warn("%s; continuing because fail_on_error is false", msg)
	return nil
}

// prepareGuest uploads the scripts, detects the guest and classifies the
// release.
//
// Returns:
//   - error: The upload or detection failed, the session is not elevated,
//     PowerShell is not 64-bit, or the OS is not supported.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) prepareGuest(ctx context.Context) error {
	r.ui.Say("Uploading the windows-optimize guest scripts...")
	if err := r.remote.install(ctx); err != nil {
		return err
	}
	r.ui.Say("Detecting the operating system, WinRM and power configuration...")
	data, err := r.remote.action(ctx, "Detect", nil)
	if err != nil {
		return err
	}
	f, err := facts.Parse(data)
	if err != nil {
		return err
	}
	r.facts = f
	for _, e := range f.Errors {
		r.warn("detection: %s", e)
	}
	if !f.Session.IsElevated {
		return fmt.Errorf("the WinRM session (%s) is not elevated; windows-optimize requires an administrator session", f.Session.UserName)
	}
	if !f.OS.Is64BitProcess {
		return errors.New("the guest PowerShell process is not 64-bit; 32-bit processes see redirected registry views")
	}
	release, err := facts.Classify(f.OS)
	if err != nil {
		return fmt.Errorf("cannot optimize %s: %w", f.OS.ProductName, err)
	}
	r.release = &release
	r.ui.Say(fmt.Sprintf("Detected %s (%s, %s, build %s, %s)", release.Name, f.OS.EditionID, release.Installation, f.OS.Version(), f.OS.Architecture))
	if !release.Known {
		r.warn("%s is newer than the releases this plugin version was tested with; optimizations whose build range covers it are applied", release.Name)
	}
	return nil
}

// inspect runs the Inspect action for the targets of changes.
//
// Returns:
//   - plan.States: The state of each target.
//   - error: The action failed or its result cannot be decoded.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) inspect(ctx context.Context, changes []*plan.ChangeItem) (plan.States, error) {
	req := make([]targetRequest, len(changes))
	for i, c := range changes {
		req[i] = targetRequest{ID: c.ID(), Target: c.Change.Target}
	}
	if len(req) == 0 {
		return plan.States{}, nil
	}
	data, err := r.remote.action(ctx, "Inspect", req)
	if err != nil {
		return nil, err
	}
	return plan.ParseStates(data)
}

// applyAndInspect applies changes and re-inspects their targets.
//
// Returns:
//   - plan.ApplyResults: The guest's result for each change.
//   - plan.States: The state of each target afterwards.
//   - error: The Apply or Inspect action failed. Per-change failures are in
//     the results instead.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) applyAndInspect(ctx context.Context, changes []*plan.ChangeItem) (plan.ApplyResults, plan.States, error) {
	req := make([]applyRequest, len(changes))
	for i, c := range changes {
		req[i] = applyRequest{ID: c.ID(), Target: c.Change.Target, Want: c.Change.Want}
	}
	data, err := r.remote.action(ctx, "Apply", req)
	if err != nil {
		return nil, nil, err
	}
	results, err := plan.ParseApplyResults(data)
	if err != nil {
		return nil, nil, err
	}
	after, err := r.inspect(ctx, changes)
	if err != nil {
		return nil, nil, err
	}
	return results, after, nil
}

// recordJournal adds the original values of pending to the rollback journal on
// the guest before anything is changed, so a failure halfway through can
// still be rolled back.
//
// Returns:
//   - error: The journal cannot be read or written. Nothing is applied then.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) recordJournal(ctx context.Context, pending []*plan.ChangeItem) error {
	journal, err := r.readJournal(ctx)
	if err != nil {
		return fmt.Errorf("%w; no changes were made", err)
	}
	r.journal.Added = journal.Record(pending, r.now())
	if err := r.writeJournal(ctx, journal); err != nil {
		return fmt.Errorf("%w; no changes were made", err)
	}
	return nil
}

// readJournal runs the ReadJournal action and decodes the journal.
//
// Returns:
//   - *plan.Journal: The journal, empty when the guest has none.
//   - error: The action failed or the journal cannot be decoded.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) readJournal(ctx context.Context) (*plan.Journal, error) {
	data, err := r.remote.action(ctx, "ReadJournal", nil)
	if err != nil {
		return nil, fmt.Errorf("read the rollback journal: %w", err)
	}
	var res struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("decode the ReadJournal result: %w", err)
	}
	r.journal.Path = res.Path
	return plan.ParseJournal([]byte(res.Content))
}

// writeJournal uploads journal and makes it the guest's rollback journal. An
// empty journal removes the file.
//
// Returns:
//   - error: The upload or the WriteJournal action failed.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) writeJournal(ctx context.Context, journal *plan.Journal) error {
	req := struct {
		Source string `json:"source"`
	}{}
	if len(journal.Entries) > 0 {
		source, err := r.remote.upload(fmt.Sprintf("journal-%02d.json", r.remote.seq+1), journal)
		if err != nil {
			return fmt.Errorf("write the rollback journal: %w", err)
		}
		req.Source = source
	}
	if _, err := r.remote.action(ctx, "WriteJournal", req); err != nil {
		return fmt.Errorf("write the rollback journal: %w", err)
	}
	r.journal.Entries = len(journal.Entries)
	return nil
}

// checkConnectivity runs a new WinRM command and compares the WinRM
// configuration with the one found by Detect, recording the outcome as a
// report check. when describes the moment, for example "after restart".
//
// Returns:
//   - error: The probe failed or WinRM regressed.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) checkConnectivity(ctx context.Context, when string) error {
	name := "new WinRM command " + when
	probe, err := r.probe(ctx)
	if err != nil {
		r.addCheck(name, false, err.Error())
		return fmt.Errorf("WinRM connectivity check %s failed: %w", when, err)
	}
	if problems := facts.WinRMRegressions(r.facts.WinRM, probe); len(problems) > 0 {
		detail := strings.Join(problems, "; ")
		r.addCheck(name, false, detail)
		return fmt.Errorf("WinRM configuration regressed %s: %s", when, detail)
	}
	r.addCheck(name, true, "a new command ran and the WinRM service, listeners and authentication are unchanged")
	return nil
}

// probe runs the Probe action.
//
// Returns:
//   - *facts.Probe: The probe result.
//   - error: The action failed or its result cannot be decoded.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) probe(ctx context.Context) (*facts.Probe, error) {
	data, err := r.remote.action(ctx, "Probe", nil)
	if err != nil {
		return nil, err
	}
	return facts.ParseProbe(data)
}

// addCheck records a connectivity check for the report.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) addCheck(name string, passed bool, details string) {
	r.checks = append(r.checks, report.Check{Name: name, At: r.now().UTC(), Passed: passed, Details: details})
}
