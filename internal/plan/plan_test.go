package plan

import (
	"slices"
	"strings"
	"testing"

	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/catalog"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/facts"
)

const balanced = "381b4222-f694-41f0-9685-ff5bb260df2e"

var (
	server2022 = facts.Release{Name: "Windows Server 2022", Installation: facts.InstallationServerDesktop, Build: 20348, Edition: "serverdatacenter", Known: true}
	win11      = facts.Release{Name: "Windows 11 version 24H2", Installation: facts.InstallationClient, Build: 26100, Edition: "enterprise", Known: true}
)

// guestFacts returns facts for a guest with the Balanced and, when
// highPerformance is set, High performance schemes installed.
//
// Author: Lars Krogh
// Version: 1.0.0
func guestFacts(highPerformance bool) *facts.Facts {
	schemes := []string{balanced}
	if highPerformance {
		schemes = append(schemes, catalog.SchemeHighPerformance)
	}
	return &facts.Facts{
		Session: facts.Session{UserName: "packer", IsElevated: true},
		WinRM:   &facts.WinRM{ServiceStatus: "Running", ServiceStart: "Automatic", Dependencies: []string{"HTTP", "RPCSS"}},
		Power:   &facts.Power{ActiveScheme: balanced, Schemes: schemes},
	}
}

// item returns the plan item with the given optimization ID.
//
// Author: Lars Krogh
// Version: 1.0.0
func item(t *testing.T, p *Plan, id string) *Item {
	t.Helper()
	for _, it := range p.Items {
		if it.Optimization.ID == id {
			return it
		}
	}
	t.Fatalf("%s is not in the plan", id)
	return nil
}

// ids returns the optimization IDs in the plan.
//
// Author: Lars Krogh
// Version: 1.0.0
func ids(p *Plan) []string {
	var out []string
	for _, it := range p.Items {
		out = append(out, it.Optimization.ID)
	}
	return out
}

// str returns a pointer to s.
//
// Author: Lars Krogh
// Version: 1.0.0
func str(s string) *string { return &s }

// stateOf returns a state that holds v.
//
// Author: Lars Krogh
// Version: 1.0.0
func stateOf(id string, v catalog.Value) State {
	st := State{ID: id, Present: v.Present}
	if v.Type != "" {
		st.Type = str(v.Type)
	}
	if v.Present {
		st.Data = str(v.Data)
	}
	return st
}

// statesFor returns states in which every change of changes holds the value
// value returns for it.
//
// Author: Lars Krogh
// Version: 1.0.0
func statesFor(changes []*ChangeItem, value func(*ChangeItem) catalog.Value) States {
	out := States{}
	for _, c := range changes {
		out[strings.ToLower(c.ID())] = stateOf(c.ID(), value(c))
	}
	return out
}

// original returns a plausible pre-optimization value for c that differs from
// the wanted value.
//
// Author: Lars Krogh
// Version: 1.0.0
func original(c *ChangeItem) catalog.Value {
	switch c.Change.Target.Kind {
	case catalog.KindRegistry:
		return catalog.Value{}
	case catalog.KindService:
		return catalog.Value{Present: true, Data: catalog.ServiceManual}
	case catalog.KindScheduledTask:
		return catalog.Value{Present: true, Data: catalog.TaskEnabled}
	case catalog.KindPowerSetting:
		return catalog.Value{Present: true, Data: "1800"}
	case catalog.KindPowerScheme:
		return catalog.Value{Present: true, Data: balanced}
	case catalog.KindHibernation:
		return catalog.Value{Present: true, Data: catalog.HibernationEnabled}
	}
	return catalog.Value{}
}

// wanted returns the value c wants.
//
// Author: Lars Krogh
// Version: 1.0.0
func wanted(c *ChangeItem) catalog.Value { return c.Change.Want }

// okResults returns a successful apply result for every change.
//
// Author: Lars Krogh
// Version: 1.0.0
func okResults(changes []*ChangeItem) ApplyResults {
	out := ApplyResults{}
	for _, c := range changes {
		out[strings.ToLower(c.ID())] = ApplyResult{ID: c.ID(), OK: true}
	}
	return out
}

// TestProfileSelection checks that each profile selects its own and the
// smaller profiles' optimizations, never explicit-only ones, and that the
// category filter applies.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProfileSelection(t *testing.T) {
	all := catalog.All()
	count := func(opts Options) int { return len(Build(all, opts, server2022, guestFacts(true)).Items) }
	recommended := count(Options{Profile: catalog.ProfileRecommended})
	performance := count(Options{Profile: catalog.ProfilePerformance})
	vdi := count(Options{Profile: catalog.ProfileVDI})
	if recommended == 0 || recommended >= performance || performance >= vdi {
		t.Errorf("profiles are not strictly cumulative: recommended %d, performance %d, vdi %d", recommended, performance, vdi)
	}
	for _, it := range Build(all, Options{Profile: catalog.ProfileVDI}, server2022, guestFacts(true)).Items {
		if it.Optimization.Profile == catalog.ProfileNone {
			t.Errorf("vdi selected explicit-only %s", it.Optimization.ID)
		}
	}
	p := Build(all, Options{Profile: catalog.ProfileVDI, Categories: []catalog.Category{catalog.CategoryPower}}, server2022, guestFacts(true))
	for _, it := range p.Items {
		if it.Optimization.Category != catalog.CategoryPower {
			t.Errorf("category filter let %s through", it.Optimization.ID)
		}
	}
	if count(Options{Profile: catalog.ProfileCustom}) != 0 {
		t.Error("custom without include must select nothing")
	}
}

// TestIncludeExclude checks include patterns, exclude precedence, and that a
// high-risk optimization is only selected by its exact ID.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestIncludeExclude(t *testing.T) {
	all := catalog.All()
	p := Build(all, Options{Profile: catalog.ProfileCustom, Include: []string{"storage.*"}}, server2022, guestFacts(true))
	got := ids(p)
	if slices.Contains(got, "storage.new_disks_online") {
		t.Error("a wildcard selected the high-risk storage.new_disks_online")
	}
	if !slices.Contains(got, "storage.short_names_off") || !slices.Contains(got, "storage.system_restore_off") {
		t.Errorf("storage.* selected %v", got)
	}
	p = Build(all, Options{Profile: catalog.ProfileCustom, Include: []string{"STORAGE.NEW_DISKS_ONLINE"}}, server2022, guestFacts(true))
	if !slices.Equal(ids(p), []string{"storage.new_disks_online"}) {
		t.Errorf("exact include selected %v", ids(p))
	}
	p = Build(all, Options{Profile: catalog.ProfileRecommended, Exclude: []string{"power.*"}, Include: []string{"power.no_disk_idle"}}, server2022, guestFacts(true))
	for _, it := range p.Items {
		excluded := strings.HasPrefix(it.Optimization.ID, "power.")
		if excluded != (it.Status == StatusSkipped) {
			t.Errorf("%s: status %s, excluded %v", it.Optimization.ID, it.Status, excluded)
		}
		if excluded && !strings.Contains(it.Reason, "excluded") {
			t.Errorf("%s: reason %q does not say it was excluded", it.Optimization.ID, it.Reason)
		}
	}
	if it := item(t, p, "power.no_disk_idle"); it.SelectedBy != "include power.no_disk_idle" {
		t.Errorf("selected by %q", it.SelectedBy)
	}
}

// TestApplicability checks that optimizations for other installation types
// or builds are unsupported with a reason, and that their changes are never
// inspected.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestApplicability(t *testing.T) {
	all := catalog.All()
	p := Build(all, Options{Profile: catalog.ProfileVDI}, win11, guestFacts(true))
	for id, wantStatus := range map[string]Status{
		"services.xbox_server":                           StatusUnsupported,
		"ux.server_manager_at_logon":                     StatusUnsupported,
		"diagnostics.shutdown_event_tracker_off":         StatusUnsupported,
		"ux.news_and_interests":                          StatusUnsupported,
		"ux.widgets":                                     StatusPending,
		"services.xbox_client":                           StatusPending,
		"ux.consumer_features":                           StatusPending,
		"windows_update.exclude_drivers":                 StatusPending,
		"windows_update.delivery_optimization_http_only": StatusPending,
	} {
		if it := item(t, p, id); it.Status != wantStatus {
			t.Errorf("%s on Windows 11: status %s (%s), want %s", id, it.Status, it.Reason, wantStatus)
		}
	}
	pro := win11
	pro.Edition = "professional"
	if it := item(t, Build(all, Options{Profile: catalog.ProfileRecommended}, pro, guestFacts(true)), "ux.consumer_features"); it.Status != StatusUnsupported {
		t.Errorf("ux.consumer_features on Pro: %s, Windows ignores the policy there", it.Status)
	}
	for _, c := range p.Pending() {
		if c.Status != ChangePending {
			t.Errorf("Pending returned %s in status %s", c.ID(), c.Status)
		}
		for _, it := range p.Items {
			if it.Status == StatusUnsupported && it.Optimization.ID == c.OptimizationID {
				t.Errorf("unsupported %s would be inspected", c.OptimizationID)
			}
		}
	}
}

// TestPowerSchemeResolution checks that power settings are written to the
// scheme that will be active: High performance when the plan activates it and
// it is installed, the active scheme otherwise.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestPowerSchemeResolution(t *testing.T) {
	all := catalog.All()
	schemeOf := func(p *Plan) string {
		return item(t, p, "power.no_idle_sleep").Changes[0].Change.Target.Scheme
	}
	if got := schemeOf(Build(all, Options{Profile: catalog.ProfilePerformance}, server2022, guestFacts(true))); got != catalog.SchemeHighPerformance {
		t.Errorf("with High performance pending, settings go to %s", got)
	}
	if got := schemeOf(Build(all, Options{Profile: catalog.ProfileRecommended}, server2022, guestFacts(true))); got != balanced {
		t.Errorf("without a scheme change, settings go to %s", got)
	}
	p := Build(all, Options{Profile: catalog.ProfilePerformance}, server2022, guestFacts(false))
	if got := schemeOf(p); got != balanced {
		t.Errorf("with High performance missing, settings go to %s", got)
	}
	if it := item(t, p, "power.high_performance_scheme"); it.Status != StatusUnsupported {
		t.Errorf("missing High performance scheme: status %s", it.Status)
	}
	excluded := Build(all, Options{Profile: catalog.ProfilePerformance, Exclude: []string{"power.high_performance_scheme"}}, server2022, guestFacts(true))
	if got := schemeOf(excluded); got != balanced {
		t.Errorf("with the scheme change excluded, settings go to %s", got)
	}
	unknown := guestFacts(true)
	unknown.Power = nil
	p = Build(all, Options{Profile: catalog.ProfilePerformance}, server2022, unknown)
	for _, id := range []string{"power.no_idle_sleep", "power.high_performance_scheme", "power.no_disk_idle"} {
		if it := item(t, p, id); it.Status != StatusSkipped {
			t.Errorf("%s with unknown power configuration: %s", id, it.Status)
		}
	}
	if it := item(t, p, "power.hibernation_off"); it.Status != StatusPending {
		t.Errorf("hibernation does not need the power schemes, got %s", it.Status)
	}
}

// TestProtectSkipsWholeOptimization checks that a protected change skips the
// whole optimization and that the dynamic WinRM rule fails closed.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProtectSkipsWholeOptimization(t *testing.T) {
	o := catalog.Optimization{
		ID:       "services.example",
		Category: catalog.CategoryServices,
		Profile:  catalog.ProfileRecommended,
		Applies:  catalog.Applicability{Installations: []facts.Installation{facts.InstallationServerDesktop}},
		Changes: []catalog.Change{
			{Target: catalog.Target{Kind: catalog.KindService, Name: "XblGameSave"}, Want: catalog.Value{Present: true, Data: catalog.ServiceDisabled}},
			{Target: catalog.Target{Kind: catalog.KindService, Name: "Harmless"}, Want: catalog.Value{Present: true, Data: catalog.ServiceDisabled}},
		},
	}
	f := guestFacts(true)
	f.WinRM.Dependencies = append(f.WinRM.Dependencies, "harmless")
	p := Build([]catalog.Optimization{o}, Options{Profile: catalog.ProfileRecommended}, server2022, f)
	it := p.Items[0]
	if it.Status != StatusSkipped || !strings.Contains(it.Reason, "WinRM service depends on Harmless") {
		t.Errorf("WinRM dependency: status %s, reason %q", it.Status, it.Reason)
	}
	if len(p.Pending()) != 0 {
		t.Error("a skipped optimization must not be inspected or applied")
	}
	f.WinRM = nil
	if it := Build([]catalog.Optimization{o}, Options{Profile: catalog.ProfileRecommended}, server2022, f).Items[0]; it.Status != StatusSkipped {
		t.Errorf("unknown WinRM dependencies must fail closed, got %s", it.Status)
	}
}

// TestProtectStatic checks representative static rules.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProtectStatic(t *testing.T) {
	protected := []catalog.Change{
		{Target: catalog.Target{Kind: catalog.KindService, Name: "WinRM"}, Want: catalog.Value{Present: true, Data: catalog.ServiceDisabled}},
		{Target: catalog.Target{Kind: catalog.KindService, Name: "MpsSvc"}, Want: catalog.Value{Present: true, Data: catalog.ServiceManual}},
		{Target: catalog.Target{Kind: catalog.KindService, Name: "WinDefend"}, Want: catalog.Value{Present: true, Data: catalog.ServiceDisabled}},
		{Target: catalog.Target{Kind: catalog.KindService, Name: "wuauserv"}, Want: catalog.Value{Present: true, Data: catalog.ServiceDisabled}},
		{Target: catalog.Target{Kind: catalog.KindService, Name: "EventLog"}, Want: catalog.Value{Present: true, Data: catalog.ServiceDisabled}},
		{Target: catalog.Target{Kind: catalog.KindRegistry, Path: `SOFTWARE\Policies\Microsoft\Windows\WinRM\Service`, Name: "AllowBasic"}, Want: catalog.Value{Present: true, Type: catalog.RegDWORD, Data: "0"}},
		{Target: catalog.Target{Kind: catalog.KindRegistry, Path: `SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`, Name: "EnableLUA"}, Want: catalog.Value{Present: true, Type: catalog.RegDWORD, Data: "0"}},
		{Target: catalog.Target{Kind: catalog.KindRegistry, Path: `SOFTWARE\Policies\Microsoft\Windows Defender`, Name: "DisableAntiSpyware"}, Want: catalog.Value{Present: true, Type: catalog.RegDWORD, Data: "1"}},
		{Target: catalog.Target{Kind: catalog.KindRegistry, Path: `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters`, Name: "X"}, Want: catalog.Value{Present: true, Type: catalog.RegDWORD, Data: "1"}},
		{Target: catalog.Target{Kind: catalog.KindScheduledTask, Path: `\Microsoft\Windows\WindowsUpdate\`, Name: "Scheduled Start"}, Want: catalog.Value{Present: true, Data: catalog.TaskDisabled}},
		{Target: catalog.Target{Kind: catalog.KindScheduledTask, Path: `\`, Name: "packer-elevated-shell"}, Want: catalog.Value{Present: true, Data: catalog.TaskDisabled}},
		{Target: catalog.Target{Kind: catalog.KindPowerSetting, Subgroup: catalog.SubgroupSleep, Setting: catalog.SettingStandbyIdle, PowerSource: "ac"}, Want: catalog.Value{Present: true, Data: "600"}},
	}
	for _, c := range protected {
		if ProtectStatic(c) == "" {
			t.Errorf("%s = %s is not protected", c.Target.ID(), c.Want.Data)
		}
	}
	never := catalog.Change{Target: catalog.Target{Kind: catalog.KindPowerSetting, Subgroup: catalog.SubgroupSleep, Setting: catalog.SettingStandbyIdle, PowerSource: "ac"}, Want: catalog.Value{Present: true, Data: "0"}}
	if why := ProtectStatic(never); why != "" {
		t.Errorf("never sleeping is safe, got %q", why)
	}
}

// TestApplyCycleAndIdempotency runs evaluate, verify and a second evaluation
// over the whole vdi profile and checks that a second run finds everything
// compliant and changes nothing.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestApplyCycleAndIdempotency(t *testing.T) {
	all := catalog.All()
	opts := Options{Profile: catalog.ProfileVDI}
	p := Build(all, opts, server2022, guestFacts(true))
	targets := p.Pending()
	p.Evaluate(statesFor(targets, original))
	pending := p.Pending()
	if len(pending) == 0 {
		t.Fatal("nothing to apply")
	}
	for _, c := range pending {
		if c.Restore == nil {
			t.Errorf("%s has no restore value", c.ID())
		}
	}
	p.Verify(okResults(pending), statesFor(pending, wanted))
	if p.Count(StatusFailed) != 0 || p.Count(StatusPending) != 0 || p.Count(StatusApplied) == 0 {
		t.Errorf("after verify: applied %d, pending %d, failed %d", p.Count(StatusApplied), p.Count(StatusPending), p.Count(StatusFailed))
	}
	needsRestart := slices.ContainsFunc(p.Items, func(it *Item) bool { return it.Status == StatusApplied && it.Optimization.RebootRequired })
	if p.RebootRequired() != needsRestart {
		t.Errorf("RebootRequired() = %v, applied optimizations need a restart: %v", p.RebootRequired(), needsRestart)
	}

	second := Build(all, opts, server2022, guestFacts(true))
	second.Evaluate(statesFor(second.Pending(), wanted))
	if n := len(second.Pending()); n != 0 {
		t.Errorf("second run would apply %d changes", n)
	}
	if second.Count(StatusCompliant) != p.Count(StatusApplied) {
		t.Errorf("second run: %d compliant, first run applied %d", second.Count(StatusCompliant), p.Count(StatusApplied))
	}
	if second.RebootRequired() {
		t.Error("a run that changes nothing must not ask for a restart")
	}
}

// TestEvaluateOutcomes checks the item status for missing targets, inspection
// failures, partial compliance and values that cannot be restored.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestEvaluateOutcomes(t *testing.T) {
	build := func() (*Plan, *Item) {
		p := Build(catalog.All(), Options{Profile: catalog.ProfileCustom, Include: []string{"services.xbox_server"}}, server2022, guestFacts(true))
		return p, p.Items[0]
	}

	p, it := build()
	p.Evaluate(statesFor(p.Pending(), func(*ChangeItem) catalog.Value { return catalog.Value{} }))
	if it.Status != StatusUnsupported {
		t.Errorf("no target exists: %s", it.Status)
	}

	p, it = build()
	states := statesFor(p.Pending(), original)
	first := strings.ToLower(it.Changes[0].ID())
	broken := states[first]
	broken.Error = str("access denied")
	states[first] = broken
	p.Evaluate(states)
	if it.Status != StatusFailed || len(p.Pending()) != 0 {
		t.Errorf("inspection failure: %s with %d pending", it.Status, len(p.Pending()))
	}

	p, it = build()
	states = statesFor(p.Pending(), original)
	delete(states, first)
	p.Evaluate(states)
	if it.Status != StatusFailed {
		t.Errorf("missing inspection result: %s", it.Status)
	}

	p, it = build()
	states = statesFor(p.Pending(), wanted)
	states[first] = stateOf(it.Changes[0].ID(), catalog.Value{Present: true, Data: "boot"})
	p.Evaluate(states)
	if it.Status != StatusSkipped || len(p.Pending()) != 0 {
		t.Errorf("unrestorable value: %s with %d pending", it.Status, len(p.Pending()))
	}

	p, it = build()
	states = statesFor(p.Pending(), wanted)
	states[first] = stateOf(it.Changes[0].ID(), original(it.Changes[0]))
	p.Evaluate(states)
	if it.Status != StatusPending || len(p.Pending()) != 1 {
		t.Errorf("one change pending: %s with %d pending", it.Status, len(p.Pending()))
	}
}

// TestVerifyRequiresReinspection checks that a change is only applied when the
// guest reported success and the value read back matches.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestVerifyRequiresReinspection(t *testing.T) {
	cases := map[string]func(pending []*ChangeItem) (ApplyResults, States){
		"guest failure": func(pending []*ChangeItem) (ApplyResults, States) {
			r := okResults(pending)
			r[strings.ToLower(pending[0].ID())] = ApplyResult{ID: pending[0].ID(), OK: false, Error: str("denied")}
			return r, statesFor(pending, wanted)
		},
		"value not written": func(pending []*ChangeItem) (ApplyResults, States) {
			return okResults(pending), statesFor(pending, original)
		},
		"no result": func(pending []*ChangeItem) (ApplyResults, States) {
			return ApplyResults{}, statesFor(pending, wanted)
		},
		"not re-inspected": func(pending []*ChangeItem) (ApplyResults, States) {
			return okResults(pending), States{}
		},
	}
	for name, outcome := range cases {
		p := Build(catalog.All(), Options{Profile: catalog.ProfileCustom, Include: []string{"telemetry.ceip_tasks"}}, server2022, guestFacts(true))
		p.Evaluate(statesFor(p.Pending(), original))
		pending := p.Pending()
		results, after := outcome(pending)
		p.Verify(results, after)
		if p.Items[0].Status != StatusFailed {
			t.Errorf("%s: status %s", name, p.Items[0].Status)
		}
		if len(p.Verified()) != 0 {
			t.Errorf("%s: a failed item must not be re-verified after restart", name)
		}
	}
}

// TestReverify checks that a value reverted by a restart fails the item.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestReverify(t *testing.T) {
	p := Build(catalog.All(), Options{Profile: catalog.ProfileCustom, Include: []string{"storage.short_names_off"}}, server2022, guestFacts(true))
	p.Evaluate(statesFor(p.Pending(), original))
	pending := p.Pending()
	p.Verify(okResults(pending), statesFor(pending, wanted))
	if !p.RebootRequired() {
		t.Error("storage.short_names_off takes effect after a restart")
	}
	verified := p.Verified()
	if len(verified) != len(pending) {
		t.Fatalf("verified %d of %d", len(verified), len(pending))
	}
	p.Reverify(statesFor(verified, wanted), "after restart")
	if p.Items[0].Status != StatusApplied {
		t.Errorf("unchanged after restart: %s", p.Items[0].Status)
	}
	p.Reverify(statesFor(verified, original), "after restart")
	if p.Items[0].Status != StatusFailed || !strings.Contains(p.Items[0].Changes[0].Reason, "after restart") {
		t.Errorf("reverted after restart: %s, %q", p.Items[0].Status, p.Items[0].Changes[0].Reason)
	}
}

// TestCompare checks value comparison per kind.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestCompare(t *testing.T) {
	reg := catalog.Change{Target: catalog.Target{Kind: catalog.KindRegistry, Path: "P", Name: "N"}, Want: catalog.Value{Present: true, Type: catalog.RegDWORD, Data: "1"}}
	absent := catalog.Change{Target: reg.Target, Want: catalog.Value{}}
	svc := catalog.Change{Target: catalog.Target{Kind: catalog.KindService, Name: "S"}, Want: catalog.Value{Present: true, Data: catalog.ServiceDisabled}}
	scheme := catalog.Change{Target: catalog.Target{Kind: catalog.KindPowerScheme}, Want: catalog.Value{Present: true, Data: catalog.SchemeHighPerformance}}
	idx := catalog.Change{Target: catalog.Target{Kind: catalog.KindPowerSetting}, Want: catalog.Value{Present: true, Data: "0"}}
	tests := []struct {
		name   string
		change catalog.Change
		state  catalog.Value
		want   Comparison
	}{
		{"dword equal", reg, catalog.Value{Present: true, Type: catalog.RegDWORD, Data: "1"}, Match},
		{"dword leading zero", reg, catalog.Value{Present: true, Type: catalog.RegDWORD, Data: "01"}, Match},
		{"dword differs", reg, catalog.Value{Present: true, Type: catalog.RegDWORD, Data: "2"}, Mismatch},
		{"wrong type", reg, catalog.Value{Present: true, Type: catalog.RegSZ, Data: "1"}, Mismatch},
		{"missing value", reg, catalog.Value{}, Mismatch},
		{"absent wanted absent", absent, catalog.Value{}, Match},
		{"absent wanted present", absent, catalog.Value{Present: true, Type: catalog.RegDWORD, Data: "1"}, Mismatch},
		{"service equal", svc, catalog.Value{Present: true, Data: catalog.ServiceDisabled}, Match},
		{"service missing", svc, catalog.Value{}, NotApplicable},
		{"scheme case", scheme, catalog.Value{Present: true, Data: strings.ToUpper(catalog.SchemeHighPerformance)}, Match},
		{"index missing", idx, catalog.Value{}, NotApplicable},
		{"index garbage", idx, catalog.Value{Present: true, Data: "x"}, Mismatch},
	}
	for _, tc := range tests {
		if got := Compare(tc.change, stateOf("x", tc.state)); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestMatchPattern checks wildcard matching.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern, id string
		want        bool
	}{
		{"ux.widgets", "ux.widgets", true},
		{"UX.Widgets", "ux.widgets", true},
		{"ux.*", "ux.widgets", true},
		{"*.widgets", "ux.widgets", true},
		{"*", "ux.widgets", true},
		{"u*s", "ux.widgets", true},
		{"ux.*_*", "ux.widgets", false},
		{"ux", "ux.widgets", false},
		{"ux.widget", "ux.widgets", false},
		{"power.*", "ux.widgets", false},
	}
	for _, tc := range tests {
		if got := MatchPattern(tc.pattern, tc.id); got != tc.want {
			t.Errorf("MatchPattern(%q, %q) = %v", tc.pattern, tc.id, got)
		}
	}
}
