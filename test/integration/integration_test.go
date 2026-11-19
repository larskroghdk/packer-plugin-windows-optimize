//go:build integration

// Package integration runs the plugin with Packer against a real Windows VM.
// See docs/testing.md for the environment variables and the authorization
// that the changing phases require.
package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/report"
)

type harness struct {
	t          *testing.T
	pluginPath string
	host       string
	username   string
	password   string
	useSSL     bool
	reports    string
}

// newHarness builds the plugin, installs it into a private plugin directory,
// and reads the target from the environment. It skips the test when
// WO_TEST_HOST is not set.
//
// Author: Lars Krogh
// Version: 1.0.0
func newHarness(t *testing.T) *harness {
	host := os.Getenv("WO_TEST_HOST")
	if host == "" {
		t.Skip("set WO_TEST_HOST and WO_TEST_PASSWORD to run the live integration tests")
	}
	password := os.Getenv("WO_TEST_PASSWORD")
	if password == "" {
		t.Fatal("WO_TEST_PASSWORD is required")
	}
	username := os.Getenv("WO_TEST_USERNAME")
	if username == "" {
		username = "Administrator"
	}
	useSSL, _ := strconv.ParseBool(os.Getenv("WO_TEST_USE_SSL"))
	dir := t.TempDir()
	binary := filepath.Join(dir, "packer-plugin-windows-optimize")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	h := &harness{t: t, pluginPath: filepath.Join(dir, "plugins"), host: host, username: username, password: password, useSSL: useSSL, reports: filepath.Join(dir, "reports")}
	h.command("go", "build", "-o", binary, "../..")
	h.command("packer", "plugins", "install", "--path", binary, "github.com/larskroghdk/windows-optimize")
	return h
}

// command runs a command with the private plugin directory and fails the test
// when it fails.
//
// Author: Lars Krogh
// Version: 1.0.0
func (h *harness) command(name string, args ...string) {
	h.t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "PACKER_PLUGIN_PATH="+h.pluginPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		h.t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

// build runs the integration template and returns the report it wrote.
//
// Parameters:
//   - name: Names the report file, for example "apply".
//   - vars: Template variables in addition to the connection settings.
//
// Returns:
//   - *report.Report: The decoded report.
//
// Author: Lars Krogh
// Version: 1.0.0
func (h *harness) build(name string, vars map[string]string) *report.Report {
	h.t.Helper()
	path := filepath.Join(h.reports, name+".json")
	args := []string{"build", "-color=false",
		"-var", "host=" + h.host,
		"-var", "username=" + h.username,
		"-var", "password=" + h.password,
		"-var", "use_ssl=" + strconv.FormatBool(h.useSSL),
		"-var", "report_path=" + filepath.ToSlash(path),
	}
	for k, v := range vars {
		args = append(args, "-var", k+"="+v)
	}
	h.command("packer", append(args, ".")...)
	data, err := os.ReadFile(path)
	if err != nil {
		h.t.Fatal(err)
	}
	var r report.Report
	if err := json.Unmarshal(data, &r); err != nil {
		h.t.Fatal(err)
	}
	h.t.Logf("%s: %s %s, summary %+v, rollback %+v", name, r.System.Release, r.Mode, r.Summary, r.Rollback)
	return &r
}

// checkpoint creates a Hyper-V checkpoint of WO_TEST_HYPERV_VM, when set, so
// the changes made by the test can be undone outside the plugin.
//
// Author: Lars Krogh
// Version: 1.0.0
func checkpoint(t *testing.T) {
	vm := os.Getenv("WO_TEST_HYPERV_VM")
	if vm == "" {
		return
	}
	name := "windows-optimize-integration-" + time.Now().Format("20060102-150405")
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		"Checkpoint-VM -Name $env:WO_TEST_HYPERV_VM -SnapshotName '"+name+"' -ErrorAction Stop").CombinedOutput()
	if err != nil {
		t.Fatalf("Checkpoint-VM %s: %v\n%s", vm, err, out)
	}
	t.Logf("created checkpoint %s of %s", name, vm)
}

// TestDryRun plans the vdi profile on the target without changing it.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestDryRun(t *testing.T) {
	h := newHarness(t)
	r := h.build("dry-run", map[string]string{"profile": "vdi", "dry_run": "true"})
	if r.Mode != report.ModeDryRun || r.Summary.Failed != 0 || r.Summary.Applied != 0 {
		t.Errorf("dry run: mode %s, summary %+v", r.Mode, r.Summary)
	}
	if len(r.System.DetectionErrors) != 0 {
		t.Errorf("detection errors: %v", r.System.DetectionErrors)
	}
}

// TestApplyIdempotentRollback applies a profile, applies it again, and rolls
// it back. It changes the target and needs WO_TEST_ALLOW_CHANGES=yes.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestApplyIdempotentRollback(t *testing.T) {
	if os.Getenv("WO_TEST_ALLOW_CHANGES") != "yes" {
		t.Skip("this test changes the target; set WO_TEST_ALLOW_CHANGES=yes for a disposable VM")
	}
	profile := os.Getenv("WO_TEST_PROFILE")
	if profile == "" {
		profile = "vdi"
	}
	h := newHarness(t)
	checkpoint(t)

	first := h.build("apply", map[string]string{"profile": profile, "dry_run": "false"})
	if first.Summary.Failed != 0 || first.Summary.Pending != 0 {
		t.Fatalf("apply: %+v", first.Summary)
	}
	for _, c := range first.Connectivity {
		if !c.Passed {
			t.Errorf("connectivity check %q failed: %s", c.Name, c.Details)
		}
	}

	second := h.build("apply-again", map[string]string{"profile": profile, "dry_run": "false"})
	if second.Summary.Applied != 0 || second.Summary.Compliant != first.Summary.Applied+first.Summary.Compliant {
		t.Errorf("second apply is not idempotent: %+v after %+v", second.Summary, first.Summary)
	}

	undo := h.build("rollback", map[string]string{"rollback": "true", "dry_run": "false"})
	if undo.Rollback == nil || undo.Rollback.Failed != 0 || undo.Rollback.Pending != 0 {
		t.Fatalf("rollback: %+v", undo.Rollback)
	}
	if undo.Journal.Entries != 0 {
		t.Errorf("%d journal entries remain after rollback", undo.Journal.Entries)
	}
}
