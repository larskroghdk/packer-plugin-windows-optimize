package plan

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/catalog"
)

// appliedPlan builds and evaluates a plan for the include patterns against
// guest values from original.
//
// Returns:
//   - *Plan: The evaluated plan.
//   - []*ChangeItem: Its pending changes.
//
// Author: Lars Krogh
// Version: 1.0.0
func appliedPlan(include ...string) (*Plan, []*ChangeItem) {
	p := Build(catalog.All(), Options{Profile: catalog.ProfileCustom, Include: include}, server2022, guestFacts(true))
	p.Evaluate(statesFor(p.Pending(), original))
	return p, p.Pending()
}

// TestParseJournal checks the empty journal and the schema version check.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestParseJournal(t *testing.T) {
	j, err := ParseJournal([]byte("  "))
	if err != nil || j.SchemaVersion != JournalSchemaVersion || j.Entries == nil {
		t.Fatalf("empty journal: %+v, %v", j, err)
	}
	if _, err := ParseJournal([]byte(`{"schema_version":99,"entries":[]}`)); err == nil {
		t.Error("an unknown schema version was accepted")
	}
	if _, err := ParseJournal([]byte(`{`)); err == nil {
		t.Error("invalid JSON was accepted")
	}
}

// TestRecordKeepsEarliestOriginal checks that a target recorded by an earlier
// run keeps its original value, and that only changes with a restore value
// are recorded.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestRecordKeepsEarliestOriginal(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	_, pending := appliedPlan("services.server_consumer")
	j, _ := ParseJournal(nil)
	if added := j.Record(pending, now); added != len(pending) {
		t.Fatalf("added %d of %d", added, len(pending))
	}
	first := j.Entries[0]
	if first.Original.Data != catalog.ServiceManual || first.Optimization != "services.server_consumer" || !first.RecordedAt.Equal(now) {
		t.Errorf("entry: %+v", first)
	}

	later := *pending[0]
	later.Restore = &catalog.Value{Present: true, Data: catalog.ServiceDisabled}
	if added := j.Record([]*ChangeItem{&later}, now.Add(time.Hour)); added != 0 {
		t.Errorf("a known target was recorded again")
	}
	if j.Entries[0].Original.Data != catalog.ServiceManual {
		t.Errorf("the earliest original was overwritten with %s", j.Entries[0].Original.Data)
	}

	unrestorable := *pending[0]
	unrestorable.Restore = nil
	unrestorable.Change.Target.Name = "Other"
	if added := j.Record([]*ChangeItem{&unrestorable}, now); added != 0 {
		t.Error("a change without a restore value was recorded")
	}
}

// TestJournalRoundTrip checks that the journal survives JSON encoding with
// every target field, which the guest needs to restore it.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestJournalRoundTrip(t *testing.T) {
	_, pending := appliedPlan("power.no_idle_sleep", "power.high_performance_scheme", "ux.server_manager_at_logon")
	j, _ := ParseJournal(nil)
	j.Record(pending, time.Now())
	data, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseJournal(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Entries) != len(j.Entries) {
		t.Fatalf("%d entries after round trip, want %d", len(back.Entries), len(j.Entries))
	}
	for i, e := range back.Entries {
		if e.Target != j.Entries[i].Target || e.Original != j.Entries[i].Original {
			t.Errorf("entry %d changed: %+v", i, e)
		}
		if e.Target.Kind == catalog.KindPowerSetting && e.Target.Scheme != catalog.SchemeHighPerformance {
			t.Errorf("%s: the journal must record the resolved scheme", e.ID)
		}
	}
}

// TestRollback runs a rollback over a journal in which one target is already
// back at its original value, one is gone, one restores and one fails, and
// checks the outcome and the remaining journal.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestRollback(t *testing.T) {
	_, pending := appliedPlan("telemetry.ceip_tasks")
	if len(pending) != 4 {
		t.Fatalf("telemetry.ceip_tasks has %d changes, the test expects 4", len(pending))
	}
	j, _ := ParseJournal(nil)
	j.Record(pending, time.Now())

	r := NewRollback(j, catalog.All())
	states := statesFor(r.Changes, func(c *ChangeItem) catalog.Value { return catalog.Value{Present: true, Data: catalog.TaskDisabled} })
	compliant, gone, restored, failing := r.Changes[0], r.Changes[1], r.Changes[2], r.Changes[3]
	states[strings.ToLower(compliant.ID())] = stateOf(compliant.ID(), catalog.Value{Present: true, Data: catalog.TaskEnabled})
	states[strings.ToLower(gone.ID())] = stateOf(gone.ID(), catalog.Value{})
	r.Evaluate(states)
	if compliant.Status != ChangeCompliant || gone.Status != ChangeNotApplicable {
		t.Fatalf("evaluate: %s, %s", compliant.Status, gone.Status)
	}
	toRestore := r.Pending()
	if len(toRestore) != 2 {
		t.Fatalf("%d pending, want 2", len(toRestore))
	}
	for _, c := range toRestore {
		if c.Change.Want.Data != catalog.TaskEnabled {
			t.Errorf("%s would be restored to %s", c.ID(), c.Change.Want.Data)
		}
	}
	results := okResults(toRestore)
	results[strings.ToLower(failing.ID())] = ApplyResult{ID: failing.ID(), OK: false, Error: str("access denied")}
	r.Verify(results, statesFor(toRestore, func(c *ChangeItem) catalog.Value { return c.Change.Want }))
	if restored.Status != ChangeRestored || failing.Status != ChangeFailed {
		t.Fatalf("verify: %s, %s", restored.Status, failing.Status)
	}
	remaining := r.Remaining(j)
	if len(remaining.Entries) != 1 || !strings.EqualFold(remaining.Entries[0].ID, failing.ID()) {
		t.Errorf("remaining journal: %+v", remaining.Entries)
	}
	if r.Count(ChangeRestored) != 1 || r.Count(ChangeFailed) != 1 {
		t.Errorf("counts: restored %d, failed %d", r.Count(ChangeRestored), r.Count(ChangeFailed))
	}

	r.Reverify(statesFor(r.Restored(), func(*ChangeItem) catalog.Value { return catalog.Value{Present: true, Data: catalog.TaskDisabled} }), "after restart")
	if restored.Status != ChangeFailed {
		t.Errorf("a restore reverted by the restart is %s", restored.Status)
	}
}

// TestRollbackRestoresAbsentRegistryValue checks that a registry value that
// did not exist before is deleted on rollback.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestRollbackRestoresAbsentRegistryValue(t *testing.T) {
	_, pending := appliedPlan("ux.server_manager_at_logon")
	j, _ := ParseJournal(nil)
	j.Record(pending, time.Now())
	r := NewRollback(j, catalog.All())
	c := r.Changes[0]
	if c.Change.Want.Present {
		t.Fatalf("rollback wants %s, the value was absent before", c.Change.Want)
	}
	r.Evaluate(statesFor(r.Changes, func(c *ChangeItem) catalog.Value {
		return catalog.Value{Present: true, Type: catalog.RegDWORD, Data: "1"}
	}))
	if c.Status != ChangePending {
		t.Fatalf("an existing value must be pending removal, got %s", c.Status)
	}
	r.Verify(okResults(r.Pending()), statesFor(r.Pending(), func(*ChangeItem) catalog.Value { return catalog.Value{} }))
	if c.Status != ChangeRestored {
		t.Errorf("after removal: %s (%s)", c.Status, c.Reason)
	}
}

// TestRollbackRejectsUnknownTargets checks that rollback restores power
// settings recorded for the resolved scheme, but refuses an entry for a target
// no catalog optimization changes and an entry whose ID does not match its
// target.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestRollbackRejectsUnknownTargets(t *testing.T) {
	_, pending := appliedPlan("power.no_idle_sleep")
	if len(pending) == 0 || pending[0].Change.Target.Scheme == "" {
		t.Fatal("power.no_idle_sleep must resolve to a scheme")
	}
	j, _ := ParseJournal(nil)
	j.Record(pending, time.Now())
	winrm := catalog.Target{Kind: catalog.KindService, Name: "WinRM"}
	j.Entries = append(j.Entries,
		JournalEntry{ID: winrm.ID(), Target: winrm, Original: catalog.Value{Present: true, Data: "disabled"}},
		JournalEntry{ID: "service:mapsbroker", Target: winrm, Original: catalog.Value{Present: true, Data: "disabled"}},
	)

	r := NewRollback(j, catalog.All())
	if got := len(r.Pending()); got != len(pending) {
		t.Errorf("%d pending, want the %d power settings", got, len(pending))
	}
	for _, c := range r.Changes[len(pending):] {
		if c.Status != ChangeFailed || c.Reason == "" {
			t.Errorf("%s is %s (%s), want failed", c.ID(), c.Status, c.Reason)
		}
	}
	r.Evaluate(statesFor(r.Pending(), original))
	if got := len(r.Remaining(j).Entries); got != 2 {
		t.Errorf("%d entries remain, want the 2 refused ones", got)
	}
}
