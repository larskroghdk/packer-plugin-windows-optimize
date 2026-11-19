package catalog

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/facts"
)

var (
	idPattern          = regexp.MustCompile(`^[a-z_]+\.[a-z0-9_]+$`)
	guidPattern        = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	serviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.\-]+$`)
	idPrefixCategory   = map[string]Category{
		"power":          CategoryPower,
		"services":       CategoryServices,
		"tasks":          CategoryScheduledTasks,
		"telemetry":      CategoryTelemetry,
		"ux":             CategoryUserExperience,
		"storage":        CategoryStorage,
		"windows_update": CategoryWindowsUpdate,
		"diagnostics":    CategoryDiagnostics,
	}
)

// Validate checks every optimization and the catalog as a whole: unique IDs
// whose prefix matches the category, complete documentation, a profile that
// matches the classification, well-formed changes, and no target that two
// optimizations could both set on the same system.
//
// Returns:
//   - error: Every problem found, joined.
//
// Author: Lars Krogh
// Version: 1.0.0
func Validate(all []Optimization) error {
	var errs []error
	seen := map[string]bool{}
	for _, o := range all {
		if seen[o.ID] {
			errs = append(errs, fmt.Errorf("%s: duplicate ID", o.ID))
		}
		seen[o.ID] = true
		for _, err := range validateOptimization(o) {
			errs = append(errs, fmt.Errorf("%s: %w", o.ID, err))
		}
	}
	errs = append(errs, checkSharedTargets(all)...)
	return errors.Join(errs...)
}

// validateOptimization checks one optimization on its own.
//
// Returns:
//   - []error: The problems found.
//
// Author: Lars Krogh
// Version: 1.0.0
func validateOptimization(o Optimization) []error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	if !idPattern.MatchString(o.ID) {
		add("ID must look like category.name in lower case")
	} else if prefix, _, _ := strings.Cut(o.ID, "."); idPrefixCategory[prefix] != o.Category {
		add("ID prefix %q does not match category %q", prefix, o.Category)
	}
	if o.Name == "" || o.Description == "" || o.Impact == "" {
		add("name, description and impact are required")
	}
	if len(o.References) == 0 {
		add("at least one reference is required")
	}
	for _, r := range o.References {
		if r.Title == "" || !strings.HasPrefix(r.URL, "https://") {
			add("reference %q needs a title and an https URL", r.URL)
		}
	}
	switch o.Classification {
	case ClassRecommended:
		if o.Profile != ProfileRecommended {
			add("a recommended optimization belongs to the recommended profile")
		}
	case ClassOptional:
		if o.Profile == ProfileRecommended {
			add("an optional optimization cannot be in the recommended profile")
		}
	case ClassHighRisk:
		if o.Profile != ProfileNone {
			add("a high-risk optimization must not be in any profile")
		}
	default:
		add("unknown classification %q", o.Classification)
	}
	switch o.Profile {
	case ProfileNone, ProfileRecommended, ProfilePerformance, ProfileVDI:
	default:
		add("unknown profile %q", o.Profile)
	}
	if len(o.Applies.Installations) == 0 {
		add("at least one installation type is required")
	}
	if o.Applies.MaxBuild > 0 && o.Applies.MaxBuild < o.Applies.MinBuild {
		add("max build %d is below min build %d", o.Applies.MaxBuild, o.Applies.MinBuild)
	}
	if len(o.Changes) == 0 {
		add("at least one change is required")
	}
	targets := map[string]bool{}
	for _, c := range o.Changes {
		id := c.Target.ID()
		if targets[id] {
			add("target %s is set twice", id)
		}
		targets[id] = true
		if err := validateChange(c); err != nil {
			add("%s: %v", id, err)
		}
	}
	return errs
}

// validateChange checks that a change's target and value are well formed for
// its kind.
//
// Returns:
//   - error: The first problem found, or nil.
//
// Author: Lars Krogh
// Version: 1.0.0
func validateChange(c Change) error {
	t, v := c.Target, c.Want
	if !v.Present {
		return errors.New("catalog changes must set a value, not remove one")
	}
	switch t.Kind {
	case KindRegistry:
		if t.Path == "" || t.Name == "" || strings.HasPrefix(t.Path, `\`) || strings.HasSuffix(t.Path, `\`) || strings.Contains(strings.ToUpper(t.Path), "HKEY_") || strings.HasPrefix(strings.ToUpper(t.Path), `HKLM\`) {
			return errors.New("registry path must be a key below HKLM without hive prefix or surrounding backslashes, and the value needs a name")
		}
		switch v.Type {
		case RegDWORD:
			if _, err := strconv.ParseUint(v.Data, 10, 32); err != nil {
				return fmt.Errorf("REG_DWORD data %q is not an unsigned 32-bit decimal", v.Data)
			}
		case RegSZ:
		default:
			return fmt.Errorf("registry type %q is not REG_DWORD or REG_SZ", v.Type)
		}
	case KindService:
		if !serviceNamePattern.MatchString(t.Name) {
			return fmt.Errorf("invalid service name %q", t.Name)
		}
		if !slices.Contains([]string{ServiceAutomatic, ServiceAutomaticDelayed, ServiceManual, ServiceDisabled}, v.Data) {
			return fmt.Errorf("invalid service start type %q", v.Data)
		}
	case KindScheduledTask:
		if !strings.HasPrefix(t.Path, `\`) || !strings.HasSuffix(t.Path, `\`) || t.Name == "" || strings.Contains(t.Name, `\`) {
			return errors.New(`task path must start and end with '\' and the name must not contain '\'`)
		}
		if v.Data != TaskEnabled && v.Data != TaskDisabled {
			return fmt.Errorf("invalid task state %q", v.Data)
		}
	case KindPowerSetting:
		if t.Scheme != "" {
			return errors.New("the scheme is resolved by the plan and must be empty in the catalog")
		}
		if !guidPattern.MatchString(t.Subgroup) || !guidPattern.MatchString(t.Setting) {
			return errors.New("subgroup and setting must be lower-case GUIDs")
		}
		if t.PowerSource != PowerSourceAC && t.PowerSource != PowerSourceDC {
			return fmt.Errorf("power source %q is not ac or dc", t.PowerSource)
		}
		if _, err := strconv.ParseUint(v.Data, 10, 32); err != nil {
			return fmt.Errorf("power index %q is not an unsigned 32-bit decimal", v.Data)
		}
	case KindPowerScheme:
		if !guidPattern.MatchString(v.Data) {
			return fmt.Errorf("power scheme %q is not a lower-case GUID", v.Data)
		}
	case KindHibernation:
		if v.Data != HibernationEnabled && v.Data != HibernationDisabled {
			return fmt.Errorf("invalid hibernation state %q", v.Data)
		}
	default:
		return fmt.Errorf("unknown kind %q", t.Kind)
	}
	return nil
}

// checkSharedTargets reports targets that two optimizations set while both can
// apply to the same system. Such a pair would fight over the value and make
// rollback ambiguous.
//
// Returns:
//   - []error: One error per shared target.
//
// Author: Lars Krogh
// Version: 1.0.0
func checkSharedTargets(all []Optimization) []error {
	var errs []error
	for i, a := range all {
		for _, b := range all[i+1:] {
			if !overlap(a.Applies, b.Applies) {
				continue
			}
			for _, ca := range a.Changes {
				for _, cb := range b.Changes {
					if ca.Target.ID() == cb.Target.ID() {
						errs = append(errs, fmt.Errorf("%s and %s both set %s on the same systems", a.ID, b.ID, ca.Target.ID()))
					}
				}
			}
		}
	}
	return errs
}

// overlap reports whether some system satisfies both applicability rules.
//
// Author: Lars Krogh
// Version: 1.0.0
func overlap(a, b Applicability) bool {
	installations := slices.ContainsFunc(a.Installations, func(i facts.Installation) bool { return slices.Contains(b.Installations, i) })
	if !installations {
		return false
	}
	upper := func(limit int) int {
		if limit == 0 {
			return int(^uint(0) >> 1)
		}
		return limit
	}
	if max(a.MinBuild, b.MinBuild) > min(upper(a.MaxBuild), upper(b.MaxBuild)) {
		return false
	}
	if len(a.Editions) == 0 || len(b.Editions) == 0 {
		return true
	}
	return slices.ContainsFunc(a.Editions, func(e string) bool { return slices.Contains(b.Editions, e) })
}
