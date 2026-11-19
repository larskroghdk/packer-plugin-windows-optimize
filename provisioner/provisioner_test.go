package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/catalog"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/facts"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/plan"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/report"
)

// provision prepares a provisioner with cfg and runs it against g.
//
// Returns:
//   - *report.Report: The report the run wrote.
//   - error: The error Provision returned.
//
// Author: Lars Krogh
// Version: 1.0.0
func provision(t *testing.T, g *fakeGuest, cfg map[string]any) (*report.Report, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.json")
	raw := map[string]any{"report_path": path}
	for k, v := range cfg {
		raw[k] = v
	}
	var p Provisioner
	if err := p.Prepare(raw); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	var out bytes.Buffer
	ui := &packer.BasicUi{Reader: strings.NewReader(""), Writer: &out, ErrorWriter: &out}
	runErr := p.Provision(context.Background(), ui, g, map[string]any{"ConnType": "winrm"})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the report was not written: %v\n%s", err, out.String())
	}
	var r report.Report
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	return &r, runErr
}

// optimization returns the report entry for id.
//
// Author: Lars Krogh
// Version: 1.0.0
func optimization(t *testing.T, r *report.Report, id string) report.Optimization {
	t.Helper()
	for _, o := range r.Optimizations {
		if o.ID == id {
			return o
		}
	}
	t.Fatalf("%s is not in the report", id)
	return report.Optimization{}
}

// TestProvisionDryRunChangesNothing checks that a dry run inspects but never
// writes the journal or applies anything.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionDryRunChangesNothing(t *testing.T) {
	g := newFakeGuest(t)
	r, err := provision(t, g, map[string]any{"profile": "vdi", "dry_run": true})
	if err != nil {
		t.Fatal(err)
	}
	if g.count("Apply") != 0 || g.count("WriteJournal") != 0 || len(g.values) != 0 {
		t.Errorf("dry run changed the guest: %v", g.actions)
	}
	if r.Mode != report.ModeDryRun || r.Summary.Pending == 0 || r.Summary.Applied != 0 {
		t.Errorf("report: mode %s, summary %+v", r.Mode, r.Summary)
	}
}

// TestProvisionApplyIsIdempotent applies the vdi profile, then runs it again
// and checks that the second run changes nothing and the journal keeps the
// first run's originals.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionApplyIsIdempotent(t *testing.T) {
	g := newFakeGuest(t)
	r, err := provision(t, g, map[string]any{"profile": "vdi"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Summary.Applied == 0 || r.Summary.Failed != 0 || r.Summary.Pending != 0 {
		t.Fatalf("first run: %+v", r.Summary)
	}
	if r.Journal.Added == 0 || r.Journal.Entries != r.Journal.Added {
		t.Errorf("journal after first run: %+v", r.Journal)
	}
	if len(r.Connectivity) != 1 || !r.Connectivity[0].Passed {
		t.Errorf("connectivity: %+v", r.Connectivity)
	}
	if g.facts.Power.ActiveScheme != catalog.SchemeHighPerformance {
		t.Errorf("active scheme %s", g.facts.Power.ActiveScheme)
	}
	journal := string(g.journal)

	applies := g.count("Apply")
	second, err := provision(t, g, map[string]any{"profile": "vdi"})
	if err != nil {
		t.Fatal(err)
	}
	if g.count("Apply") != applies {
		t.Error("the second run applied changes")
	}
	if second.Summary.Compliant != r.Summary.Applied || second.Summary.Applied != 0 {
		t.Errorf("second run: %+v, first applied %d", second.Summary, r.Summary.Applied)
	}
	if string(g.journal) != journal {
		t.Error("the second run rewrote the journal")
	}
	if second.Reboot.Required {
		t.Error("a run that changes nothing asked for a restart")
	}
}

// TestProvisionRollback applies the vdi profile, rolls it back, and checks
// that every target holds its original value and the journal is removed.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionRollback(t *testing.T) {
	g := newFakeGuest(t)
	before := map[string]catalog.Value{}
	applied, err := provision(t, g, map[string]any{"profile": "vdi"})
	if err != nil {
		t.Fatal(err)
	}
	j, err := plan.ParseJournal(g.journal)
	if err != nil {
		t.Fatal(err)
	}
	fresh := newFakeGuest(t)
	for _, e := range j.Entries {
		before[e.ID] = fresh.value(e.Target)
	}

	dry, err := provision(t, g, map[string]any{"rollback": true, "dry_run": true})
	if err != nil {
		t.Fatal(err)
	}
	if dry.Mode != report.ModeRollbackDryRun || dry.Rollback.Pending != len(j.Entries) || len(g.journal) == 0 {
		t.Fatalf("rollback dry run: %+v", dry.Rollback)
	}

	r, err := provision(t, g, map[string]any{"rollback": true})
	if err != nil {
		t.Fatal(err)
	}
	if r.Rollback == nil || r.Rollback.Restored != len(j.Entries) || r.Rollback.Failed != 0 {
		t.Fatalf("rollback: %+v (applied %d)", r.Rollback, applied.Summary.Applied)
	}
	for _, e := range j.Entries {
		if got := g.value(e.Target); got != before[e.ID] {
			t.Errorf("%s is %s after rollback, was %s", e.ID, got, before[e.ID])
		}
	}
	if len(g.journal) != 0 {
		t.Errorf("journal after a complete rollback: %s", g.journal)
	}
	if again, err := provision(t, g, map[string]any{"rollback": true}); err != nil || again.Rollback.Entries != 0 {
		t.Errorf("rollback with an empty journal: %v, %+v", err, again.Rollback)
	}
}

// TestProvisionRollbackKeepsFailedEntries checks that entries that cannot be
// restored stay in the journal and fail the build.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionRollbackKeepsFailedEntries(t *testing.T) {
	g := newFakeGuest(t)
	if _, err := provision(t, g, map[string]any{"profile": "recommended"}); err != nil {
		t.Fatal(err)
	}
	g.failApply["hibernation"] = "powercfg.exe failed"
	r, err := provision(t, g, map[string]any{"rollback": true})
	if err == nil || !strings.Contains(err.Error(), "kept in the journal") {
		t.Fatalf("error %v", err)
	}
	j, _ := plan.ParseJournal(g.journal)
	if r.Rollback.Failed != 1 || len(j.Entries) != 1 || j.Entries[0].ID != "hibernation" {
		t.Errorf("rollback %+v, remaining %+v", r.Rollback, j.Entries)
	}
}

// TestProvisionFailures checks that a change the guest rejects or does not
// persist fails the build, unless fail_on_error is false.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionFailures(t *testing.T) {
	mapsBroker := "service:mapsbroker"
	g := newFakeGuest(t)
	g.failApply[mapsBroker] = "Access is denied"
	r, err := provision(t, g, map[string]any{"profile": "vdi"})
	if err == nil || !strings.Contains(err.Error(), "1 optimizations failed") {
		t.Fatalf("error %v", err)
	}
	if o := optimization(t, r, "services.server_consumer"); o.Status != string(plan.StatusFailed) || !strings.Contains(o.Changes[0].Reason, "Access is denied") {
		t.Errorf("services.server_consumer: %+v", o)
	}
	if r.Summary.Applied == 0 {
		t.Error("one failure must not stop the other optimizations")
	}

	g = newFakeGuest(t)
	g.ignoreApply[mapsBroker] = true
	r, err = provision(t, g, map[string]any{"profile": "vdi", "fail_on_error": false})
	if err != nil {
		t.Fatalf("fail_on_error = false: %v", err)
	}
	if o := optimization(t, r, "services.server_consumer"); o.Status != string(plan.StatusFailed) || !strings.Contains(o.Changes[0].Reason, "verification failed") {
		t.Errorf("a change the guest reports but does not persist must fail verification: %+v", o)
	}
	if len(r.Warnings) == 0 {
		t.Error("the failure must be reported as a warning")
	}
}

// TestProvisionDetectsWinRMRegression checks that a change that breaks WinRM
// fails the build even with fail_on_error false.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionDetectsWinRMRegression(t *testing.T) {
	g := newFakeGuest(t)
	g.breakWinRM = func(w *facts.WinRM) { w.Auth.Basic = false }
	r, err := provision(t, g, map[string]any{"profile": "recommended", "fail_on_error": false})
	if err == nil || !strings.Contains(err.Error(), "WinRM Basic authentication was disabled") {
		t.Fatalf("error %v", err)
	}
	if len(r.Connectivity) != 1 || r.Connectivity[0].Passed {
		t.Errorf("connectivity: %+v", r.Connectivity)
	}
}

// TestProvisionProtectsWinRMDependencies checks that a service WinRM depends
// on is never changed, even when a profile selects it.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionProtectsWinRMDependencies(t *testing.T) {
	g := newFakeGuest(t)
	g.facts.WinRM.Dependencies = append(g.facts.WinRM.Dependencies, "MapsBroker")
	r, err := provision(t, g, map[string]any{"profile": "vdi"})
	if err != nil {
		t.Fatal(err)
	}
	o := optimization(t, r, "services.server_consumer")
	if o.Status != string(plan.StatusSkipped) || !strings.Contains(o.Reason, "WinRM service depends on MapsBroker") {
		t.Errorf("services.server_consumer: %s (%s)", o.Status, o.Reason)
	}
	for _, name := range []string{"mapsbroker", "lfsvc", "icssvc", "phonesvc"} {
		if _, set := g.values["service:"+name]; set {
			t.Errorf("%s was changed although its optimization was skipped", name)
		}
	}

	g = newFakeGuest(t)
	g.facts.WinRM = nil
	g.facts.Errors = []string{"winrm: access denied"}
	if _, err := provision(t, g, map[string]any{"profile": "vdi"}); err == nil {
		t.Error("unknown WinRM configuration: the connectivity check must fail")
	}
	for id := range g.values {
		if strings.HasPrefix(id, "service:") {
			t.Errorf("%s was changed although the WinRM dependencies were unknown", id)
		}
	}
}

// TestProvisionReboot checks that a restart is only performed when
// reboot_if_required is set, and that values a restart reverts fail the
// build.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionReboot(t *testing.T) {
	cfg := map[string]any{"profile": "custom", "include": []string{"storage.short_names_off"}}
	g := newFakeGuest(t)
	r, err := provision(t, g, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Reboot.Required || r.Reboot.Performed || g.ran("shutdown.exe") {
		t.Errorf("without reboot_if_required: %+v", r.Reboot)
	}

	cfg["reboot_if_required"] = true
	g = newFakeGuest(t)
	r, err = provision(t, g, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Reboot.Performed || !g.ran("shutdown.exe") || len(r.Connectivity) != 2 {
		t.Errorf("with reboot_if_required: %+v, checks %+v", r.Reboot, r.Connectivity)
	}

	g = newFakeGuest(t)
	g.revertOnReboot[`registry:hklm\system\currentcontrolset\control\filesystem\ntfsdisable8dot3namecreation`] = true
	r, err = provision(t, g, cfg)
	if err == nil {
		t.Fatal("a value reverted by the restart must fail the build")
	}
	if o := optimization(t, r, "storage.short_names_off"); o.Status != string(plan.StatusFailed) {
		t.Errorf("storage.short_names_off: %s", o.Status)
	}
}

// TestProvisionRefusesUnsafeSessions checks the preconditions that are
// checked before anything is changed.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionRefusesUnsafeSessions(t *testing.T) {
	g := newFakeGuest(t)
	g.facts.Session.IsElevated = false
	if _, err := provision(t, g, nil); err == nil || !strings.Contains(err.Error(), "not elevated") {
		t.Errorf("non-elevated session: %v", err)
	}
	g = newFakeGuest(t)
	g.facts.OS.Is64BitProcess = false
	if _, err := provision(t, g, nil); err == nil {
		t.Error("32-bit PowerShell was accepted")
	}
	g = newFakeGuest(t)
	g.facts.OS.ProductType = facts.ProductTypeDomainController
	r, err := provision(t, g, nil)
	if err == nil || r.System.ProductName == "" {
		t.Errorf("domain controller: %v; the report must still describe the system", err)
	}
	if g.count("Apply") != 0 {
		t.Error("changes were applied to a refused system")
	}

	var p Provisioner
	if err := p.Prepare(map[string]any{"report_path": filepath.Join(t.TempDir(), "r.json")}); err != nil {
		t.Fatal(err)
	}
	if err := p.Provision(context.Background(), &packer.BasicUi{Writer: &bytes.Buffer{}, ErrorWriter: &bytes.Buffer{}}, newFakeGuest(t), map[string]any{"ConnType": "ssh"}); err == nil {
		t.Error("the SSH communicator was accepted")
	}
}

// TestProvisionUnknownRelease checks that a build newer than the release
// tables is optimized with a warning.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionUnknownRelease(t *testing.T) {
	g := newFakeGuest(t)
	g.facts.OS.Build = 26500
	r, err := provision(t, g, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.System.KnownRelease || len(r.Warnings) == 0 || r.Summary.Applied == 0 {
		t.Errorf("unknown release: known %v, warnings %v, summary %+v", r.System.KnownRelease, r.Warnings, r.Summary)
	}
}

// TestReportDescribesRun checks the report fields that people and pipelines
// read.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestReportDescribesRun(t *testing.T) {
	g := newFakeGuest(t)
	r, err := provision(t, g, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.SchemaVersion != report.SchemaVersion || r.Profile != "recommended" || r.Mode != report.ModeApply {
		t.Errorf("header: %+v", r)
	}
	if r.System.Release != "Windows Server 2022" || r.System.Installation != "server_desktop" || r.System.ActivePower != balancedScheme {
		t.Errorf("system: %+v", r.System)
	}
	if !r.FinishedAt.After(r.StartedAt) && !r.FinishedAt.Equal(r.StartedAt) {
		t.Errorf("times: %s .. %s", r.StartedAt, r.FinishedAt)
	}
	for _, o := range r.Optimizations {
		if len(o.References) == 0 || len(o.Changes) == 0 || o.SelectedBy == "" {
			t.Errorf("%s: incomplete entry %+v", o.ID, o)
		}
		for _, c := range o.Changes {
			if o.Status == string(plan.StatusApplied) && (c.Before == "" || c.After == "") {
				t.Errorf("%s: applied change %s without before and after values", o.ID, c.ID)
			}
		}
	}
	lines := strings.Join(report.Build(report.Input{Mode: report.ModeApply, Reboot: report.Reboot{Required: true, Reason: "x"}, StartedAt: time.Now(), FinishedAt: time.Now()}).SummaryLines(), "\n")
	if !strings.Contains(lines, "reboot_if_required") {
		t.Errorf("the summary must say how to restart:\n%s", lines)
	}
}
