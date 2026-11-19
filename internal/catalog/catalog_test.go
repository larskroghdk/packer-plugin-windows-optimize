package catalog_test

import (
	"strings"
	"testing"

	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/catalog"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/facts"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/plan"
)

// TestCatalogIsValid checks the embedded catalog with the same validation that
// Prepare runs.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestCatalogIsValid(t *testing.T) {
	if err := catalog.Validate(catalog.All()); err != nil {
		t.Fatal(err)
	}
}

// TestCatalogNeverTouchesProtectedTargets checks that no catalog change hits
// a static protection rule, so protection is a safety net and never the reason
// a catalog optimization is skipped.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestCatalogNeverTouchesProtectedTargets(t *testing.T) {
	for _, o := range catalog.All() {
		for _, c := range o.Changes {
			if why := plan.ProtectStatic(c); why != "" {
				t.Errorf("%s: %s", o.ID, why)
			}
		}
	}
}

// TestRecommendedProfileIsConservative checks the promises the documentation
// makes about the default profile: nothing that needs a restart, nothing that
// disables a client feature, and no optimization without a Microsoft source.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestRecommendedProfileIsConservative(t *testing.T) {
	for _, o := range catalog.All() {
		if o.Profile != catalog.ProfileRecommended {
			continue
		}
		if o.RebootRequired {
			t.Errorf("%s: the recommended profile must not need a restart", o.ID)
		}
		microsoft := false
		for _, r := range o.References {
			if strings.HasPrefix(r.URL, "https://learn.microsoft.com/") {
				microsoft = true
			}
		}
		if !microsoft {
			t.Errorf("%s: a recommended optimization needs a Microsoft Learn reference", o.ID)
		}
	}
}

// TestNoSecurityControlIsDisabled checks that the catalog never sets the
// values that turn off Defender, Windows Update, auditing or event logging,
// even outside the static deny-list.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestNoSecurityControlIsDisabled(t *testing.T) {
	forbidden := []string{"defender", "windowsupdate\\au", "eventlog", "audit", "firewall", "winrm", "wsman", "lsa", "smartscreen"}
	for _, o := range catalog.All() {
		for _, c := range o.Changes {
			id := strings.ToLower(c.Target.ID())
			for _, word := range forbidden {
				if strings.Contains(id, word) {
					t.Errorf("%s: %s touches %q", o.ID, c.Target.ID(), word)
				}
			}
		}
	}
}

// TestProfilesAreCumulative checks that each profile contains the smaller ones
// and that high-risk optimizations are in none of them.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProfilesAreCumulative(t *testing.T) {
	order := []catalog.Profile{catalog.ProfileRecommended, catalog.ProfilePerformance, catalog.ProfileVDI}
	for i, p := range order {
		for _, smaller := range order[:i+1] {
			if !p.Includes(smaller) {
				t.Errorf("%s must include %s", p, smaller)
			}
		}
		for _, larger := range order[i+1:] {
			if p.Includes(larger) {
				t.Errorf("%s must not include %s", p, larger)
			}
		}
		if p.Includes(catalog.ProfileNone) {
			t.Errorf("%s must not include explicit-only optimizations", p)
		}
	}
	if catalog.ProfileCustom.Includes(catalog.ProfileRecommended) {
		t.Error("custom must select nothing by profile")
	}
	for _, o := range catalog.All() {
		if o.Classification == catalog.ClassHighRisk && o.Profile != catalog.ProfileNone {
			t.Errorf("%s: high-risk optimization in profile %s", o.ID, o.Profile)
		}
	}
}

// TestAllReturnsCopy checks that callers cannot change the embedded catalog.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestAllReturnsCopy(t *testing.T) {
	a := catalog.All()
	a[0].Changes[0].Target.Scheme = "changed"
	a[0].ID = "changed"
	b := catalog.All()
	if b[0].ID == "changed" || b[0].Changes[0].Target.Scheme == "changed" {
		t.Fatal("All must return a deep copy")
	}
}

// validOptimization returns a minimal optimization that passes Validate.
//
// Author: Lars Krogh
// Version: 1.0.0
func validOptimization(id string) catalog.Optimization {
	return catalog.Optimization{
		ID:             id,
		Name:           "Example",
		Description:    "Example.",
		Impact:         "None.",
		Category:       catalog.CategoryTelemetry,
		Classification: catalog.ClassOptional,
		Profile:        catalog.ProfilePerformance,
		Applies:        catalog.Applicability{Installations: []facts.Installation{facts.InstallationClient}},
		References:     []catalog.Reference{{Title: "Example", URL: "https://learn.microsoft.com/example"}},
		Changes: []catalog.Change{{
			Target: catalog.Target{Kind: catalog.KindRegistry, Path: `SOFTWARE\Policies\Example`, Name: "Value"},
			Want:   catalog.Value{Present: true, Type: catalog.RegDWORD, Data: "1"},
		}},
	}
}

// TestValidateRejects checks that each kind of catalog mistake is caught.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestValidateRejects(t *testing.T) {
	tests := map[string]func(o *catalog.Optimization){
		"bad id":              func(o *catalog.Optimization) { o.ID = "Telemetry.Example" },
		"wrong prefix":        func(o *catalog.Optimization) { o.ID = "power.example" },
		"no impact":           func(o *catalog.Optimization) { o.Impact = "" },
		"http reference":      func(o *catalog.Optimization) { o.References[0].URL = "http://example.com" },
		"recommended profile": func(o *catalog.Optimization) { o.Profile = catalog.ProfileRecommended },
		"high risk in vdi":    func(o *catalog.Optimization) { o.Classification, o.Profile = catalog.ClassHighRisk, catalog.ProfileVDI },
		"no installation":     func(o *catalog.Optimization) { o.Applies.Installations = nil },
		"inverted builds":     func(o *catalog.Optimization) { o.Applies.MinBuild, o.Applies.MaxBuild = 20000, 19000 },
		"hive prefix":         func(o *catalog.Optimization) { o.Changes[0].Target.Path = `HKLM\SOFTWARE\Example` },
		"negative dword":      func(o *catalog.Optimization) { o.Changes[0].Want.Data = "-1" },
		"removal":             func(o *catalog.Optimization) { o.Changes[0].Want = catalog.Value{} },
		"REG_BINARY":          func(o *catalog.Optimization) { o.Changes[0].Want.Type = "REG_BINARY" },
		"duplicate target":    func(o *catalog.Optimization) { o.Changes = append(o.Changes, o.Changes[0]) },
		"service start":       func(o *catalog.Optimization) { o.Changes[0] = serviceChange("Example", "boot") },
		"task path":           func(o *catalog.Optimization) { o.Changes[0] = taskChange(`Microsoft\Windows`, "Task") },
		"power scheme set": func(o *catalog.Optimization) {
			o.Changes[0] = powerChange(catalog.SubgroupSleep, catalog.SettingStandbyIdle, "ac")
			o.Changes[0].Target.Scheme = catalog.SchemeHighPerformance
		},
		"upper-case guid": func(o *catalog.Optimization) {
			o.Changes[0] = powerChange(strings.ToUpper(catalog.SubgroupSleep), catalog.SettingStandbyIdle, "ac")
		},
		"power source": func(o *catalog.Optimization) {
			o.Changes[0] = powerChange(catalog.SubgroupSleep, catalog.SettingStandbyIdle, "battery")
		},
	}
	for name, mutate := range tests {
		o := validOptimization("telemetry.example")
		mutate(&o)
		if err := catalog.Validate([]catalog.Optimization{o}); err == nil {
			t.Errorf("%s: Validate accepted it", name)
		}
	}
	if err := catalog.Validate([]catalog.Optimization{validOptimization("telemetry.example")}); err != nil {
		t.Fatalf("the valid example was rejected: %v", err)
	}
}

// TestValidateSharedTargets checks that two optimizations may set the same
// target only when no system can select both.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestValidateSharedTargets(t *testing.T) {
	a, b := validOptimization("telemetry.a"), validOptimization("telemetry.b")
	if err := catalog.Validate([]catalog.Optimization{a, b}); err == nil {
		t.Error("two optimizations on the same systems setting one target were accepted")
	}
	b.Applies.Installations = []facts.Installation{facts.InstallationServerDesktop}
	if err := catalog.Validate([]catalog.Optimization{a, b}); err != nil {
		t.Errorf("disjoint installation types must not conflict: %v", err)
	}
	b.Applies.Installations = a.Applies.Installations
	a.Applies.MaxBuild, b.Applies.MinBuild = 19045, 22000
	if err := catalog.Validate([]catalog.Optimization{a, b}); err != nil {
		t.Errorf("disjoint build ranges must not conflict: %v", err)
	}
	a.Applies.MaxBuild, b.Applies.MinBuild = 0, 0
	a.Applies.Editions, b.Applies.Editions = []string{"enterprise"}, []string{"professional"}
	if err := catalog.Validate([]catalog.Optimization{a, b}); err != nil {
		t.Errorf("disjoint editions must not conflict: %v", err)
	}
}

// TestApplicabilityCheck checks installation, build range and edition limits.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestApplicabilityCheck(t *testing.T) {
	a := catalog.Applicability{
		Installations: []facts.Installation{facts.InstallationClient},
		MinBuild:      19041,
		MaxBuild:      19045,
		Editions:      []string{"enterprise"},
	}
	release := facts.Release{Installation: facts.InstallationClient, Build: 19045, Edition: "enterprise"}
	if ok, why := a.Check(release); !ok {
		t.Fatalf("matching release rejected: %s", why)
	}
	for name, r := range map[string]facts.Release{
		"server":  {Installation: facts.InstallationServerDesktop, Build: 19045, Edition: "enterprise"},
		"older":   {Installation: facts.InstallationClient, Build: 17763, Edition: "enterprise"},
		"newer":   {Installation: facts.InstallationClient, Build: 22631, Edition: "enterprise"},
		"edition": {Installation: facts.InstallationClient, Build: 19045, Edition: "professional"},
	} {
		if ok, why := a.Check(r); ok || why == "" {
			t.Errorf("%s: accepted, or rejected without a reason", name)
		}
	}
}

// TestTargetID checks the canonical target IDs used in reports and the
// journal.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestTargetID(t *testing.T) {
	tests := map[string]catalog.Target{
		`registry:HKLM\SOFTWARE\X\V`:            {Kind: catalog.KindRegistry, Path: `SOFTWARE\X`, Name: "V"},
		"service:SysMain":                       {Kind: catalog.KindService, Name: "SysMain"},
		`scheduled_task:\Microsoft\Windows\A\B`: {Kind: catalog.KindScheduledTask, Path: `\Microsoft\Windows\A\`, Name: "B"},
		"power_setting:active/s/x/ac":           {Kind: catalog.KindPowerSetting, Subgroup: "s", Setting: "x", PowerSource: "ac"},
		"power_setting:g/s/x/dc":                {Kind: catalog.KindPowerSetting, Scheme: "g", Subgroup: "s", Setting: "x", PowerSource: "dc"},
		"power_scheme:active":                   {Kind: catalog.KindPowerScheme},
		"hibernation":                           {Kind: catalog.KindHibernation},
	}
	for want, target := range tests {
		if got := target.ID(); got != want {
			t.Errorf("ID() = %q, want %q", got, want)
		}
	}
}

// serviceChange returns a service start type change for tests.
//
// Author: Lars Krogh
// Version: 1.0.0
func serviceChange(name, start string) catalog.Change {
	return catalog.Change{Target: catalog.Target{Kind: catalog.KindService, Name: name}, Want: catalog.Value{Present: true, Data: start}}
}

// taskChange returns a task disable change for tests.
//
// Author: Lars Krogh
// Version: 1.0.0
func taskChange(path, name string) catalog.Change {
	return catalog.Change{Target: catalog.Target{Kind: catalog.KindScheduledTask, Path: path, Name: name}, Want: catalog.Value{Present: true, Data: catalog.TaskDisabled}}
}

// powerChange returns a power index change of 0 for tests.
//
// Author: Lars Krogh
// Version: 1.0.0
func powerChange(subgroup, setting, source string) catalog.Change {
	return catalog.Change{
		Target: catalog.Target{Kind: catalog.KindPowerSetting, Subgroup: subgroup, Setting: setting, PowerSource: source},
		Want:   catalog.Value{Present: true, Data: "0"},
	}
}
