package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/catalog"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/facts"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/plan"
)

const balancedScheme = "381b4222-f694-41f0-9685-ff5bb260df2e"

var actionPattern = regexp.MustCompile(`-Action (\w+) -OutputPath "([^"]+)"(?: -InputPath "([^"]+)")?`)

// fakeGuest is an in-memory Windows guest behind a packer.Communicator. It
// implements the guest actions of Invoke-WindowsOptimize.ps1 over a map of
// target values, and fails the test when the plugin sends a change that
// breaks the plugin's own safety rules.
type fakeGuest struct {
	t        *testing.T
	mu       sync.Mutex
	files    map[string][]byte
	facts    facts.Facts
	values   map[string]catalog.Value
	journal  []byte
	bootTime int
	actions  []string
	commands []string

	failApply      map[string]string
	ignoreApply    map[string]bool
	revertOnReboot map[string]bool
	breakWinRM     func(*facts.WinRM)
}

// newFakeGuest returns a Windows Server 2022 Desktop Experience guest with
// both power schemes installed, Balanced active, and every target in its
// Windows default state.
//
// Author: Lars Krogh
// Version: 1.0.0
func newFakeGuest(t *testing.T) *fakeGuest {
	return &fakeGuest{
		t:     t,
		files: map[string][]byte{},
		facts: facts.Facts{
			OS: facts.OS{
				ProductName: "Windows Server 2022 Datacenter", EditionID: "ServerDatacenter", InstallationType: "Server",
				Major: 10, Build: 20348, UBR: 2700, ProductType: facts.ProductTypeServer, Architecture: "AMD64", Is64BitProcess: true,
			},
			Session: facts.Session{UserName: `BUILD\packer`, IsElevated: true},
			WinRM: &facts.WinRM{
				ServiceStatus: "Running", ServiceStart: "Automatic",
				Dependencies:     []string{"HTTP", "RPCSS", "DcomLaunch", "RpcEptMapper"},
				Listeners:        []facts.Listener{{Transport: "HTTP", Address: "*", Port: 5985, Enabled: true}},
				AllowUnencrypted: true,
				Auth:             facts.WinRMAuth{Basic: true, Negotiate: true, Kerberos: true},
			},
			Power:  &facts.Power{ActiveScheme: balancedScheme, Schemes: []string{balancedScheme, catalog.SchemeHighPerformance}},
			Errors: []string{},
		},
		values:         map[string]catalog.Value{},
		failApply:      map[string]string{},
		ignoreApply:    map[string]bool{},
		revertOnReboot: map[string]bool{},
	}
}

// value returns the current value of target, or the Windows default for its
// kind when it was never set.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) value(target catalog.Target) catalog.Value {
	if target.Kind == catalog.KindPowerScheme {
		return catalog.Value{Present: true, Data: g.facts.Power.ActiveScheme}
	}
	if v, ok := g.values[strings.ToLower(target.ID())]; ok {
		return v
	}
	switch target.Kind {
	case catalog.KindService:
		return catalog.Value{Present: true, Data: catalog.ServiceManual}
	case catalog.KindScheduledTask:
		return catalog.Value{Present: true, Data: catalog.TaskEnabled}
	case catalog.KindPowerSetting:
		return catalog.Value{Present: true, Data: "1800"}
	case catalog.KindHibernation:
		return catalog.Value{Present: true, Data: catalog.HibernationEnabled}
	}
	return catalog.Value{}
}

// set stores v as the value of target.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) set(target catalog.Target, v catalog.Value) {
	if target.Kind == catalog.KindPowerScheme {
		g.facts.Power.ActiveScheme = v.Data
		return
	}
	g.values[strings.ToLower(target.ID())] = v
}

// Start runs cmd against the fake guest and reports its exit status.
//
// Returns:
//   - error: Always nil; failures are reported through the exit status.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) Start(_ context.Context, cmd *packer.RemoteCmd) error {
	g.mu.Lock()
	g.commands = append(g.commands, cmd.Command)
	code, out := g.run(cmd.Command)
	g.mu.Unlock()
	go func() {
		if out != "" {
			fmt.Fprintln(cmd.Stdout, out)
		}
		cmd.SetExited(code)
	}()
	return nil
}

// run executes one command line.
//
// Returns:
//   - int: The exit status.
//   - string: Output for stdout.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) run(command string) (int, string) {
	switch {
	case strings.HasPrefix(command, "shutdown.exe /r"):
		g.reboot()
		return 0, ""
	case strings.Contains(command, "[IO.Directory]::CreateDirectory"), strings.Contains(command, "Remove-Item -LiteralPath"):
		return 0, ""
	}
	m := actionPattern.FindStringSubmatch(command)
	if m == nil {
		g.t.Errorf("unexpected guest command: %s", command)
		return 1, ""
	}
	action, output, input := m[1], m[2], m[3]
	g.actions = append(g.actions, action)
	result, err := g.action(action, g.files[input])
	if err != nil {
		return 1, "ERROR: " + err.Error()
	}
	data, err := json.Marshal(result)
	if err != nil {
		g.t.Fatal(err)
	}
	g.files[output] = data
	return 0, ""
}

// action runs one guest action on its decoded input.
//
// Returns:
//   - any: The result to encode as JSON.
//   - error: The input cannot be decoded or the action is unknown.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) action(name string, input []byte) (any, error) {
	switch name {
	case "Detect":
		return g.facts, nil
	case "Probe":
		probe := facts.Probe{Session: g.facts.Session, BootTime: fmt.Sprintf("2026-01-01T00:%02d:00Z", g.bootTime)}
		if g.facts.WinRM == nil {
			msg := "access denied"
			probe.Error = &msg
			return probe, nil
		}
		w := *g.facts.WinRM
		w.Listeners = append([]facts.Listener(nil), w.Listeners...)
		probe.WinRM = &w
		return probe, nil
	case "Inspect":
		var req []targetRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		items := make([]plan.State, len(req))
		for i, r := range req {
			items[i] = toState(r.ID, g.value(r.Target))
		}
		return map[string]any{"items": items}, nil
	case "Apply":
		var req []applyRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		return map[string]any{"items": g.apply(req)}, nil
	case "ReadJournal":
		return map[string]string{"path": `C:\ProgramData\packer-plugin-windows-optimize\journal.json`, "content": string(g.journal)}, nil
	case "WriteJournal":
		var req struct {
			Source string `json:"source"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		g.journal = nil
		if req.Source != "" {
			g.journal = append([]byte(nil), g.files[req.Source]...)
		}
		return map[string]string{"path": `C:\ProgramData\packer-plugin-windows-optimize\journal.json`}, nil
	}
	return nil, fmt.Errorf("unknown action %s", name)
}

// apply sets each requested target, after checking the plugin's safety rules:
// no protected or WinRM-dependency target, and every target already in the
// rollback journal.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) apply(req []applyRequest) []plan.ApplyResult {
	journal, err := plan.ParseJournal(g.journal)
	if err != nil {
		g.t.Fatalf("journal on the guest is invalid: %v", err)
	}
	originals := map[string]catalog.Value{}
	for _, e := range journal.Entries {
		originals[strings.ToLower(e.ID)] = e.Original
	}
	out := make([]plan.ApplyResult, len(req))
	for i, r := range req {
		original, journaled := originals[strings.ToLower(r.ID)]
		restore := journaled && original == r.Want
		change := catalog.Change{Target: r.Target, Want: r.Want}
		if why := plan.Protect(change, &g.facts); why != "" && !restore {
			g.t.Errorf("the plugin applied protected change %s: %s", r.ID, why)
		}
		if !journaled {
			g.t.Errorf("the plugin applied %s before recording it in the rollback journal", r.ID)
		}
		out[i] = plan.ApplyResult{ID: r.ID, OK: true}
		if msg, ok := g.failApply[strings.ToLower(r.ID)]; ok {
			out[i] = plan.ApplyResult{ID: r.ID, Error: &msg}
			continue
		}
		if !g.ignoreApply[strings.ToLower(r.ID)] {
			g.set(r.Target, r.Want)
		}
	}
	if g.breakWinRM != nil {
		g.breakWinRM(g.facts.WinRM)
	}
	return out
}

// reboot advances the boot time and reverts the targets marked to revert.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) reboot() {
	g.bootTime++
	for id := range g.revertOnReboot {
		delete(g.values, id)
	}
}

// Upload stores the uploaded file.
//
// Returns:
//   - error: Reading r failed.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) Upload(path string, r io.Reader, _ *os.FileInfo) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.files[path] = data
	return nil
}

// Download writes a stored file to w.
//
// Returns:
//   - error: The file does not exist.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) Download(path string, w io.Writer) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	data, ok := g.files[path]
	if !ok {
		return fmt.Errorf("%s does not exist", path)
	}
	_, err := io.Copy(w, bytes.NewReader(data))
	return err
}

// UploadDir is not used by the plugin.
//
// Returns:
//   - error: Always.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) UploadDir(string, string, []string) error {
	return fmt.Errorf("UploadDir is not supported")
}

// DownloadDir is not used by the plugin.
//
// Returns:
//   - error: Always.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) DownloadDir(string, string, []string) error {
	return fmt.Errorf("DownloadDir is not supported")
}

// count returns how often the plugin ran action.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) count(action string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, a := range g.actions {
		if a == action {
			n++
		}
	}
	return n
}

// ran reports whether a command starting with prefix was run.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) ran(prefix string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, c := range g.commands {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// toState converts a value to the guest's Inspect state.
//
// Author: Lars Krogh
// Version: 1.0.0
func toState(id string, v catalog.Value) plan.State {
	st := plan.State{ID: id, Present: v.Present}
	if v.Type != "" {
		st.Type = &v.Type
	}
	if v.Present {
		st.Data = &v.Data
	}
	return st
}
