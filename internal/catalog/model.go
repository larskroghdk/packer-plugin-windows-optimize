// Package catalog defines the optimization model and the embedded catalog of
// optimizations. An optimization is a named set of changes. Each change sets
// one target (a registry value, a service start type, a scheduled task state,
// a power setting, the active power scheme or hibernation) to one value, so
// every change can be inspected, applied, verified and restored the same way.
package catalog

import (
	"fmt"
	"slices"
	"strings"

	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/facts"
)

// Kind is the type of target a change sets.
type Kind string

const (
	KindRegistry      Kind = "registry"
	KindService       Kind = "service"
	KindScheduledTask Kind = "scheduled_task"
	KindPowerSetting  Kind = "power_setting"
	KindPowerScheme   Kind = "power_scheme"
	KindHibernation   Kind = "hibernation"
)

const (
	RegDWORD = "REG_DWORD"
	RegSZ    = "REG_SZ"
)

const (
	ServiceAutomatic        = "automatic"
	ServiceAutomaticDelayed = "automatic_delayed"
	ServiceManual           = "manual"
	ServiceDisabled         = "disabled"
	TaskEnabled             = "enabled"
	TaskDisabled            = "disabled"
	HibernationEnabled      = "enabled"
	HibernationDisabled     = "disabled"
	PowerSourceAC           = "ac"
	PowerSourceDC           = "dc"
)

// Target identifies the object a change sets. Which fields are used depends
// on Kind:
//   - registry: Path is the key below HKLM, Name the value name.
//   - service: Name is the service name.
//   - scheduled_task: Path is the task folder, ending in '\', Name the task.
//   - power_setting: Subgroup and Setting are GUIDs, PowerSource is "ac" or
//     "dc", and Scheme is the scheme GUID. Scheme is empty in the catalog and
//     resolved by the plan to the scheme that will be active.
//   - power_scheme and hibernation: no fields.
type Target struct {
	Kind        Kind   `json:"kind"`
	Path        string `json:"path"`
	Name        string `json:"name"`
	Scheme      string `json:"scheme"`
	Subgroup    string `json:"subgroup"`
	Setting     string `json:"setting"`
	PowerSource string `json:"power_source"`
}

// ID returns the canonical, human-readable identifier of the target. It is
// used in reports, in the rollback journal and to detect two optimizations
// that set the same target.
//
// Author: Lars Krogh
// Version: 1.0.0
func (t Target) ID() string {
	switch t.Kind {
	case KindRegistry:
		return `registry:HKLM\` + t.Path + `\` + t.Name
	case KindService:
		return "service:" + t.Name
	case KindScheduledTask:
		return "scheduled_task:" + t.Path + t.Name
	case KindPowerSetting:
		scheme := t.Scheme
		if scheme == "" {
			scheme = "active"
		}
		return fmt.Sprintf("power_setting:%s/%s/%s/%s", scheme, t.Subgroup, t.Setting, t.PowerSource)
	case KindPowerScheme:
		return "power_scheme:active"
	case KindHibernation:
		return "hibernation"
	}
	return string(t.Kind) + ":?"
}

// Value is a target's value. Present is false for a registry value that does
// not exist. Type is set for registry values only. Data is the value as text:
// a decimal number for DWORDs and power indexes, a lower-case GUID for the
// power scheme, and one of the service, task or hibernation state constants.
type Value struct {
	Present bool   `json:"present"`
	Type    string `json:"type"`
	Data    string `json:"data"`
}

// String describes the value for reports.
//
// Author: Lars Krogh
// Version: 1.0.0
func (v Value) String() string {
	switch {
	case !v.Present:
		return "(absent)"
	case v.Type != "":
		return v.Type + " " + v.Data
	}
	return v.Data
}

// Change sets one target to the wanted value.
type Change struct {
	Target Target `json:"target"`
	Want   Value  `json:"want"`
}

// Category groups optimizations by the part of Windows they change.
type Category string

const (
	CategoryPower          Category = "power"
	CategoryServices       Category = "services"
	CategoryScheduledTasks Category = "scheduled_tasks"
	CategoryTelemetry      Category = "telemetry"
	CategoryUserExperience Category = "user_experience"
	CategoryStorage        Category = "storage"
	CategoryWindowsUpdate  Category = "windows_update"
	CategoryDiagnostics    Category = "diagnostics"
)

// Categories lists every category in documentation order.
var Categories = []Category{
	CategoryPower, CategoryServices, CategoryScheduledTasks, CategoryTelemetry,
	CategoryUserExperience, CategoryStorage, CategoryWindowsUpdate, CategoryDiagnostics,
}

// Classification is the risk class of an optimization.
type Classification string

const (
	ClassRecommended Classification = "recommended"
	ClassOptional    Classification = "optional"
	ClassHighRisk    Classification = "high_risk"
)

// Profile is a named, cumulative selection of optimizations: performance
// contains recommended, and vdi contains performance. ProfileNone marks an
// optimization that is only applied when included explicitly.
type Profile string

const (
	ProfileNone        Profile = ""
	ProfileRecommended Profile = "recommended"
	ProfilePerformance Profile = "performance"
	ProfileVDI         Profile = "vdi"
	ProfileCustom      Profile = "custom"
)

// rank orders the cumulative profiles. ProfileNone and ProfileCustom rank 0.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p Profile) rank() int {
	switch p {
	case ProfileRecommended:
		return 1
	case ProfilePerformance:
		return 2
	case ProfileVDI:
		return 3
	}
	return 0
}

// Includes reports whether profile p contains an optimization whose lowest
// profile is member.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p Profile) Includes(member Profile) bool {
	return member.rank() > 0 && member.rank() <= p.rank()
}

// Applicability limits an optimization to installation types, a build range
// and editions. MaxBuild 0 means no upper limit, and an empty Editions list
// means every edition. Editions are lower-case EditionID values.
type Applicability struct {
	Installations []facts.Installation
	MinBuild      int
	MaxBuild      int
	Editions      []string
}

// Check decides whether the optimization applies to release r.
//
// Returns:
//   - bool: Whether it applies.
//   - string: Why it does not apply, or "" when it does.
//
// Author: Lars Krogh
// Version: 1.0.0
func (a Applicability) Check(r facts.Release) (bool, string) {
	if !slices.Contains(a.Installations, r.Installation) {
		return false, fmt.Sprintf("applies to %s, not %s", joinInstallations(a.Installations), r.Installation)
	}
	if a.MinBuild > 0 && r.Build < a.MinBuild {
		return false, fmt.Sprintf("requires build %d or later, the guest is build %d", a.MinBuild, r.Build)
	}
	if a.MaxBuild > 0 && r.Build > a.MaxBuild {
		return false, fmt.Sprintf("applies up to build %d, the guest is build %d", a.MaxBuild, r.Build)
	}
	if len(a.Editions) > 0 && !slices.Contains(a.Editions, r.Edition) {
		return false, fmt.Sprintf("Windows honors it only on editions %s, the guest is %s", strings.Join(a.Editions, ", "), r.Edition)
	}
	return true, ""
}

// joinInstallations returns the installation types as a comma-separated list.
//
// Author: Lars Krogh
// Version: 1.0.0
func joinInstallations(in []facts.Installation) string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return strings.Join(out, ", ")
}

// Reference is an authoritative source for an optimization.
type Reference struct {
	Title string
	URL   string
}

// Optimization is one catalog entry. Description says what it changes and
// why, Impact the security and compatibility consequences. RebootRequired
// means the change takes full effect only after a restart.
type Optimization struct {
	ID             string
	Name           string
	Description    string
	Impact         string
	Category       Category
	Classification Classification
	Profile        Profile
	Applies        Applicability
	RebootRequired bool
	References     []Reference
	Changes        []Change
}
