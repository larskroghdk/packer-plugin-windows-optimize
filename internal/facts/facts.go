// Package facts decodes what the guest reports about itself: the Windows
// release, edition and installation type, the Packer session, the WinRM
// configuration and the services it depends on, and the power schemes.
package facts

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ProductTypeWorkstation      = 1
	ProductTypeDomainController = 2
	ProductTypeServer           = 3
)

// Facts is the result of the guest Detect action. Pointer fields are nil when
// the guest could not collect that part. Errors says why, and every decision
// that needs a missing part must fail closed.
type Facts struct {
	OS      OS       `json:"os"`
	Session Session  `json:"session"`
	WinRM   *WinRM   `json:"winrm"`
	Power   *Power   `json:"power"`
	Errors  []string `json:"errors"`
}

// OS identifies the Windows release, edition, installation type and process
// architecture.
type OS struct {
	ProductName      string `json:"product_name"`
	EditionID        string `json:"edition_id"`
	InstallationType string `json:"installation_type"`
	DisplayVersion   string `json:"display_version"`
	Major            int    `json:"major"`
	Minor            int    `json:"minor"`
	Build            int    `json:"build"`
	UBR              int    `json:"ubr"`
	ProductType      int    `json:"product_type"`
	Architecture     string `json:"architecture"`
	Is64BitProcess   bool   `json:"is_64bit_process"`
}

// Session describes the account and token of the Packer WinRM session.
type Session struct {
	UserName   string `json:"user_name"`
	IsElevated bool   `json:"is_elevated"`
}

// WinRM is the WinRM service configuration. Dependencies lists every service
// the WinRM service needs, directly or indirectly.
type WinRM struct {
	ServiceStatus    string     `json:"service_status"`
	ServiceStart     string     `json:"service_start"`
	Dependencies     []string   `json:"dependencies"`
	Listeners        []Listener `json:"listeners"`
	AllowUnencrypted bool       `json:"allow_unencrypted"`
	Auth             WinRMAuth  `json:"auth"`
}

// Listener is a configured WinRM listener.
type Listener struct {
	Transport string `json:"transport"`
	Address   string `json:"address"`
	Port      int    `json:"port"`
	Enabled   bool   `json:"enabled"`
}

// WinRMAuth lists the authentication methods enabled on the WinRM service.
type WinRMAuth struct {
	Basic       bool `json:"basic"`
	Kerberos    bool `json:"kerberos"`
	Negotiate   bool `json:"negotiate"`
	Certificate bool `json:"certificate"`
	CredSSP     bool `json:"credssp"`
}

// Power lists the power schemes installed on the guest and the active one.
// GUIDs are lower case.
type Power struct {
	ActiveScheme string   `json:"active_scheme"`
	Schemes      []string `json:"schemes"`
}

// HasScheme reports whether the scheme GUID is installed, ignoring case.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Power) HasScheme(guid string) bool {
	for _, s := range p.Schemes {
		if strings.EqualFold(s, guid) {
			return true
		}
	}
	return false
}

// Parse decodes the Detect result and normalizes the power scheme GUIDs to
// lower case.
//
// Returns:
//   - *Facts: The decoded facts.
//   - error: The data is not valid JSON, or the build or product type is
//     missing, which means the guest could not identify the OS.
//
// Author: Lars Krogh
// Version: 1.0.0
func Parse(data []byte) (*Facts, error) {
	var f Facts
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("decode system facts: %w", err)
	}
	if f.OS.Build == 0 || f.OS.ProductType == 0 {
		return nil, fmt.Errorf("system facts are incomplete: build %d, product type %d", f.OS.Build, f.OS.ProductType)
	}
	if f.Power != nil {
		f.Power.ActiveScheme = strings.ToLower(f.Power.ActiveScheme)
		for i, s := range f.Power.Schemes {
			f.Power.Schemes[i] = strings.ToLower(s)
		}
	}
	return &f, nil
}

// Version returns the full version as major.minor.build.revision.
//
// Author: Lars Krogh
// Version: 1.0.0
func (o OS) Version() string { return fmt.Sprintf("%d.%d.%d.%d", o.Major, o.Minor, o.Build, o.UBR) }
