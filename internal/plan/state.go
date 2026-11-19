package plan

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/catalog"
)

// State is the guest's view of one target. Present is false when the
// registry value, service, task or power setting does not exist, and Error is
// set when the target could not be read.
type State struct {
	ID      string  `json:"id"`
	Present bool    `json:"present"`
	Type    *string `json:"type"`
	Data    *string `json:"data"`
	Error   *string `json:"error"`
}

// Value converts the state to a catalog value.
//
// Author: Lars Krogh
// Version: 1.0.0
func (s State) Value() catalog.Value {
	return catalog.Value{Present: s.Present, Type: deref(s.Type), Data: deref(s.Data)}
}

// String describes the state for reports.
//
// Author: Lars Krogh
// Version: 1.0.0
func (s State) String() string {
	if s.Error != nil {
		return "(error)"
	}
	return s.Value().String()
}

// States maps lower-case target IDs to guest state.
type States map[string]State

// Get returns the state for a target ID, matched case-insensitively.
//
// Returns:
//   - State: The state.
//   - bool: Whether the guest returned a state for the ID.
//
// Author: Lars Krogh
// Version: 1.0.0
func (s States) Get(id string) (State, bool) {
	st, ok := s[strings.ToLower(id)]
	return st, ok
}

// ParseStates decodes the Inspect result.
//
// Returns:
//   - States: The states by target ID.
//   - error: The data is not valid JSON.
//
// Author: Lars Krogh
// Version: 1.0.0
func ParseStates(data []byte) (States, error) {
	var res struct {
		Items []State `json:"items"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("decode inspect result: %w", err)
	}
	out := make(States, len(res.Items))
	for _, s := range res.Items {
		out[strings.ToLower(s.ID)] = s
	}
	return out, nil
}

// ApplyResult is the guest's result of applying one change.
type ApplyResult struct {
	ID    string  `json:"id"`
	OK    bool    `json:"ok"`
	Error *string `json:"error"`
}

// ApplyResults maps lower-case target IDs to apply results.
type ApplyResults map[string]ApplyResult

// Get returns the result for a target ID, matched case-insensitively.
//
// Returns:
//   - ApplyResult: The result.
//   - bool: Whether the guest returned a result for the ID.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r ApplyResults) Get(id string) (ApplyResult, bool) {
	res, ok := r[strings.ToLower(id)]
	return res, ok
}

// ParseApplyResults decodes the Apply result.
//
// Returns:
//   - ApplyResults: The results by target ID.
//   - error: The data is not valid JSON.
//
// Author: Lars Krogh
// Version: 1.0.0
func ParseApplyResults(data []byte) (ApplyResults, error) {
	var res struct {
		Items []ApplyResult `json:"items"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("decode apply result: %w", err)
	}
	out := make(ApplyResults, len(res.Items))
	for _, r := range res.Items {
		out[strings.ToLower(r.ID)] = r
	}
	return out, nil
}

// Comparison is the result of comparing guest state with a wanted value.
type Comparison int

const (
	Mismatch Comparison = iota
	Match
	NotApplicable
)

// Compare checks state st against the value change c wants. DWORDs and power
// indexes compare numerically, GUIDs case-insensitively and everything else
// exactly. A missing service, task, power setting or unknown hibernation state
// is NotApplicable. A missing registry value is a Mismatch, because the change
// creates it, unless the wanted value is absent too.
//
// Author: Lars Krogh
// Version: 1.0.0
func Compare(c catalog.Change, st State) Comparison {
	want, got := c.Want, st.Value()
	switch c.Target.Kind {
	case catalog.KindRegistry:
		if !want.Present || !got.Present {
			if want.Present == got.Present {
				return Match
			}
			return Mismatch
		}
		if got.Type != want.Type {
			return Mismatch
		}
		if want.Type == catalog.RegDWORD {
			return compareNumbers(want.Data, got.Data)
		}
		return matchIf(got.Data == want.Data)
	case catalog.KindPowerSetting:
		if !got.Present {
			return NotApplicable
		}
		return compareNumbers(want.Data, got.Data)
	case catalog.KindPowerScheme:
		return matchIf(strings.EqualFold(got.Data, want.Data))
	case catalog.KindService, catalog.KindScheduledTask, catalog.KindHibernation:
		if !got.Present {
			return NotApplicable
		}
		return matchIf(got.Data == want.Data)
	}
	return Mismatch
}

// compareNumbers compares two unsigned 32-bit decimals. Anything that does not
// parse is a Mismatch.
//
// Author: Lars Krogh
// Version: 1.0.0
func compareNumbers(want, got string) Comparison {
	w, err1 := strconv.ParseUint(want, 10, 32)
	g, err2 := strconv.ParseUint(got, 10, 32)
	return matchIf(err1 == nil && err2 == nil && w == g)
}

// matchIf returns Match when ok and Mismatch otherwise.
//
// Author: Lars Krogh
// Version: 1.0.0
func matchIf(ok bool) Comparison {
	if ok {
		return Match
	}
	return Mismatch
}

// restoreValue returns the value that puts a target of kind back into the
// state before. The plugin only changes what it can put back.
//
// Returns:
//   - catalog.Value: The value to restore.
//   - error: The state cannot be written back, for example a registry value
//     of a type the plugin does not write or a boot-start service.
//
// Author: Lars Krogh
// Version: 1.0.0
func restoreValue(kind catalog.Kind, before State) (catalog.Value, error) {
	v := before.Value()
	switch kind {
	case catalog.KindRegistry:
		if !v.Present || v.Type == catalog.RegDWORD || v.Type == catalog.RegSZ {
			return v, nil
		}
		return v, fmt.Errorf("registry type %s cannot be restored", v.Type)
	case catalog.KindService:
		switch v.Data {
		case catalog.ServiceAutomatic, catalog.ServiceAutomaticDelayed, catalog.ServiceManual, catalog.ServiceDisabled:
			return v, nil
		}
		return v, fmt.Errorf("start type %q cannot be restored", v.Data)
	case catalog.KindScheduledTask:
		if v.Data == catalog.TaskEnabled || v.Data == catalog.TaskDisabled {
			return v, nil
		}
		return v, fmt.Errorf("task state %q cannot be restored", v.Data)
	case catalog.KindPowerSetting:
		if _, err := strconv.ParseUint(v.Data, 10, 32); err != nil {
			return v, fmt.Errorf("power index %q cannot be restored", v.Data)
		}
		return v, nil
	case catalog.KindPowerScheme:
		if v.Data == "" {
			return v, fmt.Errorf("the active power scheme is unknown")
		}
		return v, nil
	case catalog.KindHibernation:
		if v.Data == catalog.HibernationEnabled || v.Data == catalog.HibernationDisabled {
			return v, nil
		}
		return v, fmt.Errorf("hibernation state %q cannot be restored", v.Data)
	}
	return v, fmt.Errorf("unknown kind %q", kind)
}

// deref returns the string s points to, or "" for nil.
//
// Author: Lars Krogh
// Version: 1.0.0
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
