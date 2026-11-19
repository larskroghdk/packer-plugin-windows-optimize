//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package provisioner

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/catalog"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/plan"
)

const defaultRestartTimeout = 30 * time.Minute

// Config is the provisioner's HCL configuration. See docs/configuration.md for
// each option.
type Config struct {
	common.PackerConfig `mapstructure:",squash"`
	Profile             string         `mapstructure:"profile"`
	Categories          []string       `mapstructure:"categories"`
	Include             []string       `mapstructure:"include"`
	Exclude             []string       `mapstructure:"exclude"`
	DryRun              bool           `mapstructure:"dry_run"`
	Rollback            bool           `mapstructure:"rollback"`
	FailOnError         config.Trilean `mapstructure:"fail_on_error"`
	RebootIfRequired    bool           `mapstructure:"reboot_if_required"`
	RestartTimeout      time.Duration  `mapstructure:"restart_timeout"`
	ReportPath          string         `mapstructure:"report_path"`
	ctx                 interpolate.Context
}

// prepare decodes and interpolates the raw configuration, applies defaults and
// validates every option against the catalog. Include and exclude patterns
// that match no optimization are errors, so a typo cannot silently change
// what is applied.
//
// Returns:
//   - error: All validation errors, combined.
//
// Author: Lars Krogh
// Version: 1.0.0
func (c *Config) prepare(all []catalog.Optimization, raws ...any) error {
	err := config.Decode(c, &config.DecodeOpts{
		PluginType:         "windows-optimize",
		Interpolate:        true,
		InterpolateContext: &c.ctx,
	}, raws...)
	if err != nil {
		return err
	}
	var errs error
	if c.Rollback && (c.Profile != "" || len(c.Categories) > 0 || len(c.Include) > 0 || len(c.Exclude) > 0) {
		errs = packer.MultiErrorAppend(errs, errors.New("rollback: restores every entry of the rollback journal; profile, categories, include and exclude cannot be combined with it"))
	}
	if c.Profile == "" && !c.Rollback {
		c.Profile = string(catalog.ProfileRecommended)
	}
	if c.RestartTimeout == 0 {
		c.RestartTimeout = defaultRestartTimeout
	}
	if c.ReportPath == "" {
		name := c.PackerBuildName
		if name == "" {
			name = "build"
		}
		c.ReportPath = fmt.Sprintf("windows-optimize-report-%s.json", sanitizeFileName(name))
	}
	if !c.Rollback {
		switch catalog.Profile(c.Profile) {
		case catalog.ProfileRecommended, catalog.ProfilePerformance, catalog.ProfileVDI:
		case catalog.ProfileCustom:
			if len(c.Include) == 0 {
				errs = packer.MultiErrorAppend(errs, errors.New("profile: custom selects nothing on its own; list the optimizations in include"))
			}
		default:
			errs = packer.MultiErrorAppend(errs, fmt.Errorf("profile: must be %q, %q, %q or %q", catalog.ProfileRecommended, catalog.ProfilePerformance, catalog.ProfileVDI, catalog.ProfileCustom))
		}
	}
	for i, name := range c.Categories {
		if !slices.Contains(catalog.Categories, catalog.Category(name)) {
			errs = packer.MultiErrorAppend(errs, fmt.Errorf("categories[%d]: unknown category %q; valid categories are %s", i, name, categoryList()))
		}
	}
	for i, pattern := range c.Include {
		if !matchesAny(all, pattern) {
			errs = packer.MultiErrorAppend(errs, fmt.Errorf("include[%d]: %q matches no optimization ID", i, pattern))
		}
	}
	for i, pattern := range c.Exclude {
		switch {
		case strings.Trim(pattern, "* ") == "":
			errs = packer.MultiErrorAppend(errs, fmt.Errorf("exclude[%d]: pattern %q would exclude every optimization; select a smaller profile instead", i, pattern))
		case !matchesAny(all, pattern):
			errs = packer.MultiErrorAppend(errs, fmt.Errorf("exclude[%d]: %q matches no optimization ID", i, pattern))
		}
	}
	if c.RestartTimeout < time.Minute {
		errs = packer.MultiErrorAppend(errs, errors.New("restart_timeout: must be at least 1m"))
	}
	return errs
}

// options converts the configuration to plan selection options.
//
// Author: Lars Krogh
// Version: 1.0.0
func (c *Config) options() plan.Options {
	opts := plan.Options{Profile: catalog.Profile(c.Profile), Include: c.Include, Exclude: c.Exclude}
	for _, name := range c.Categories {
		opts.Categories = append(opts.Categories, catalog.Category(name))
	}
	return opts
}

// failOnError reports whether failed optimizations fail the build. It is true
// unless fail_on_error is explicitly false.
//
// Author: Lars Krogh
// Version: 1.0.0
func (c *Config) failOnError() bool { return !c.FailOnError.False() }

// matchesAny reports whether pattern matches the ID of any optimization.
//
// Author: Lars Krogh
// Version: 1.0.0
func matchesAny(all []catalog.Optimization, pattern string) bool {
	return slices.ContainsFunc(all, func(o catalog.Optimization) bool { return plan.MatchPattern(pattern, o.ID) })
}

// categoryList returns the valid categories as a quoted, comma-separated list
// for error messages.
//
// Author: Lars Krogh
// Version: 1.0.0
func categoryList() string {
	out := make([]string, len(catalog.Categories))
	for i, c := range catalog.Categories {
		out[i] = fmt.Sprintf("%q", c)
	}
	return strings.Join(out, ", ")
}

// sanitizeFileName replaces every character other than ASCII letters, digits,
// '-', '_' and '.' with '_'.
//
// Author: Lars Krogh
// Version: 1.0.0
func sanitizeFileName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		}
		return '_'
	}, s)
}
