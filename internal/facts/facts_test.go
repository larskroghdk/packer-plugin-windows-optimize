package facts

import (
	"strings"
	"testing"
)

// TestClassify checks release names, installation types and rejections for
// every supported family and the unsupported cases.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestClassify(t *testing.T) {
	server := func(build int, installation string) OS {
		return OS{Major: 10, Build: build, ProductType: ProductTypeServer, InstallationType: installation, EditionID: "ServerDatacenter"}
	}
	client := func(build int) OS {
		return OS{Major: 10, Build: build, ProductType: ProductTypeWorkstation, InstallationType: "Client", EditionID: "Enterprise"}
	}
	tests := []struct {
		os           OS
		name         string
		installation Installation
		known        bool
	}{
		{server(14393, "Server"), "Windows Server 2016", InstallationServerDesktop, true},
		{server(14393, "Server Core"), "Windows Server 2016 (Server Core)", InstallationServerCore, true},
		{server(17763, "Server"), "Windows Server 2019", InstallationServerDesktop, true},
		{server(20348, "Server Core"), "Windows Server 2022 (Server Core)", InstallationServerCore, true},
		{server(26100, "Server"), "Windows Server 2025", InstallationServerDesktop, true},
		{server(26500, "Server"), "Windows Server build 26500", InstallationServerDesktop, false},
		{client(19045), "Windows 10 version 22H2", InstallationClient, true},
		{client(26100), "Windows 11 version 24H2", InstallationClient, true},
		{client(19046), "Windows 10 build 19046", InstallationClient, false},
		{client(27000), "Windows 11 build 27000", InstallationClient, false},
	}
	for _, tc := range tests {
		r, err := Classify(tc.os)
		if err != nil {
			t.Errorf("Classify(%d %s): %v", tc.os.Build, tc.os.InstallationType, err)
			continue
		}
		if r.Name != tc.name || r.Installation != tc.installation || r.Known != tc.known {
			t.Errorf("Classify(%d %s) = %q %s known=%v, want %q %s known=%v", tc.os.Build, tc.os.InstallationType, r.Name, r.Installation, r.Known, tc.name, tc.installation, tc.known)
		}
		if r.Edition != strings.ToLower(tc.os.EditionID) {
			t.Errorf("Classify(%d): edition %q is not lower case", tc.os.Build, r.Edition)
		}
	}
}

// TestClassifyRejects checks that unsupported systems are refused instead of
// guessed.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestClassifyRejects(t *testing.T) {
	tests := map[string]OS{
		"Windows 8.1":       {Major: 6, Minor: 3, Build: 9600, ProductType: ProductTypeWorkstation, InstallationType: "Client"},
		"domain controller": {Major: 10, Build: 20348, ProductType: ProductTypeDomainController, InstallationType: "Server"},
		"Windows 10 1507":   {Major: 10, Build: 10240, ProductType: ProductTypeWorkstation, InstallationType: "Client"},
		"Server 2012 R2":    {Major: 10, Build: 9600, ProductType: ProductTypeServer, InstallationType: "Server"},
		"Nano Server":       {Major: 10, Build: 14393, ProductType: ProductTypeServer, InstallationType: "Nano Server"},
		"unknown client":    {Major: 10, Build: 19045, ProductType: ProductTypeWorkstation, InstallationType: "Embedded"},
		"product type 0":    {Major: 10, Build: 19045},
	}
	for name, o := range tests {
		if r, err := Classify(o); err == nil {
			t.Errorf("%s: Classify accepted it as %q", name, r.Name)
		}
	}
}

// TestParse checks that GUIDs are lowered and incomplete facts are refused.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestParse(t *testing.T) {
	f, err := Parse([]byte(`{"os":{"build":20348,"product_type":3},"power":{"active_scheme":"381B4222-F694-41F0-9685-FF5BB260DF2E","schemes":["381B4222-F694-41F0-9685-FF5BB260DF2E"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Power.ActiveScheme != "381b4222-f694-41f0-9685-ff5bb260df2e" || f.Power.Schemes[0] != f.Power.ActiveScheme {
		t.Errorf("power GUIDs not lowered: %+v", f.Power)
	}
	if !f.Power.HasScheme("381B4222-F694-41F0-9685-FF5BB260DF2E") {
		t.Error("HasScheme must ignore case")
	}
	if f.WinRM != nil {
		t.Error("missing winrm must decode as nil")
	}
	for _, data := range []string{`{"os":{"product_type":3}}`, `{"os":{"build":20348}}`, `not json`} {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("Parse(%s) accepted incomplete facts", data)
		}
	}
}

// TestParseProbe checks that a probe without a boot time is refused, because
// restart detection depends on it.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestParseProbe(t *testing.T) {
	if _, err := ParseProbe([]byte(`{"session":{"is_elevated":true}}`)); err == nil {
		t.Error("probe without boot time accepted")
	}
	p, err := ParseProbe([]byte(`{"session":{"is_elevated":true},"boot_time":"2026-01-01T00:00:00Z"}`))
	if err != nil || p.BootTime == "" {
		t.Fatalf("ParseProbe: %v", err)
	}
}

// healthyWinRM returns a typical Packer WinRM configuration.
//
// Author: Lars Krogh
// Version: 1.0.0
func healthyWinRM() *WinRM {
	return &WinRM{
		ServiceStatus:    "Running",
		ServiceStart:     "Automatic",
		Dependencies:     []string{"HTTP", "RPCSS"},
		Listeners:        []Listener{{Transport: "HTTP", Address: "*", Port: 5985, Enabled: true}},
		Auth:             WinRMAuth{Basic: true, Negotiate: true, Kerberos: true},
		AllowUnencrypted: true,
	}
}

// TestWinRMRegressions checks that every kind of regression is reported and
// that additions are not.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestWinRMRegressions(t *testing.T) {
	probe := func(mutate func(*WinRM)) *Probe {
		w := healthyWinRM()
		if mutate != nil {
			mutate(w)
		}
		return &Probe{WinRM: w, Session: Session{IsElevated: true}, BootTime: "x"}
	}
	if got := WinRMRegressions(healthyWinRM(), probe(nil)); len(got) != 0 {
		t.Errorf("unchanged WinRM reported regressions: %v", got)
	}
	added := probe(func(w *WinRM) {
		w.Listeners = append(w.Listeners, Listener{Transport: "HTTPS", Address: "*", Port: 5986, Enabled: true})
		w.Auth.CredSSP = true
	})
	if got := WinRMRegressions(healthyWinRM(), added); len(got) != 0 {
		t.Errorf("additions reported as regressions: %v", got)
	}
	probeError := "boom"
	regressions := map[string]*Probe{
		"stopped":         probe(func(w *WinRM) { w.ServiceStatus = "Stopped" }),
		"start type":      probe(func(w *WinRM) { w.ServiceStart = "Manual" }),
		"listener gone":   probe(func(w *WinRM) { w.Listeners = nil }),
		"listener off":    probe(func(w *WinRM) { w.Listeners[0].Enabled = false }),
		"basic disabled":  probe(func(w *WinRM) { w.Auth.Basic = false }),
		"not elevated":    {WinRM: healthyWinRM(), Session: Session{IsElevated: false}, BootTime: "x"},
		"unreadable":      {Session: Session{IsElevated: true}, BootTime: "x"},
		"probe error":     {WinRM: healthyWinRM(), Session: Session{IsElevated: true}, BootTime: "x", Error: &probeError},
		"unencrypted off": probe(func(w *WinRM) { w.AllowUnencrypted = false }),
	}
	for name, p := range regressions {
		if got := WinRMRegressions(healthyWinRM(), p); len(got) == 0 {
			t.Errorf("%s: no regression reported", name)
		}
	}
	if got := WinRMRegressions(nil, &Probe{Session: Session{IsElevated: true}, BootTime: "x"}); len(got) != 0 {
		t.Errorf("unknown baseline must only check elevation and errors, got %v", got)
	}
}
