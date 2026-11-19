package facts

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Probe is the result of the guest Probe action, taken after a change to check
// that WinRM still works.
type Probe struct {
	WinRM    *WinRM  `json:"winrm"`
	Session  Session `json:"session"`
	BootTime string  `json:"boot_time"`
	Error    *string `json:"error"`
}

// ParseProbe decodes the Probe result.
//
// Returns:
//   - *Probe: The decoded probe.
//   - error: The data is not valid JSON or has no boot time.
//
// Author: Lars Krogh
// Version: 1.0.0
func ParseProbe(data []byte) (*Probe, error) {
	var p Probe
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("decode probe result: %w", err)
	}
	if p.BootTime == "" {
		return nil, errors.New("probe result has no boot time")
	}
	return &p, nil
}

// WinRMRegressions compares the WinRM configuration before a change with a
// probe taken afterwards. A regression is a lost elevation, a stopped service,
// a service no longer starting automatically, a missing or disabled listener,
// a disabled authentication method, or newly disallowed unencrypted traffic.
// Additions are not regressions. When before is nil, only the probe error and
// the elevation are checked.
//
// Returns:
//   - []string: One description per regression, or nil if there are none.
//
// Author: Lars Krogh
// Version: 1.0.0
func WinRMRegressions(before *WinRM, after *Probe) []string {
	var out []string
	if after.Error != nil {
		out = append(out, "WinRM state could not be read: "+*after.Error)
	}
	if !after.Session.IsElevated {
		out = append(out, "the WinRM session is no longer elevated")
	}
	if before == nil {
		return out
	}
	if after.WinRM == nil {
		return append(out, "WinRM configuration could not be read after the change")
	}
	a := after.WinRM
	if !strings.EqualFold(a.ServiceStatus, "Running") {
		out = append(out, "WinRM service status is "+a.ServiceStatus)
	}
	if strings.EqualFold(before.ServiceStart, "Automatic") && !strings.EqualFold(a.ServiceStart, "Automatic") {
		out = append(out, "WinRM service start type changed to "+a.ServiceStart)
	}
	for _, l := range before.Listeners {
		if l.Enabled && !hasListener(a.Listeners, l) {
			out = append(out, fmt.Sprintf("WinRM %s listener on port %d is missing or disabled", l.Transport, l.Port))
		}
	}
	auth := []struct {
		name          string
		before, after bool
	}{
		{"Basic", before.Auth.Basic, a.Auth.Basic},
		{"Kerberos", before.Auth.Kerberos, a.Auth.Kerberos},
		{"Negotiate", before.Auth.Negotiate, a.Auth.Negotiate},
		{"Certificate", before.Auth.Certificate, a.Auth.Certificate},
		{"CredSSP", before.Auth.CredSSP, a.Auth.CredSSP},
	}
	for _, m := range auth {
		if m.before && !m.after {
			out = append(out, fmt.Sprintf("WinRM %s authentication was disabled", m.name))
		}
	}
	if before.AllowUnencrypted && !a.AllowUnencrypted {
		out = append(out, "WinRM unencrypted traffic was disallowed")
	}
	return out
}

// hasListener reports whether listeners contains an enabled listener with the
// transport and port of want.
//
// Author: Lars Krogh
// Version: 1.0.0
func hasListener(listeners []Listener, want Listener) bool {
	for _, l := range listeners {
		if strings.EqualFold(l.Transport, want.Transport) && l.Port == want.Port && l.Enabled {
			return true
		}
	}
	return false
}
