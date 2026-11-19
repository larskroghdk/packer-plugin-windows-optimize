package provisioner

import (
	"strings"
	"testing"
	"time"

	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/catalog"
)

// prepareConfig runs Config.prepare on raw against the embedded catalog.
//
// Returns:
//   - Config: The prepared configuration.
//   - error: The validation errors.
//
// Author: Lars Krogh
// Version: 1.0.0
func prepareConfig(raw map[string]any) (Config, error) {
	var c Config
	err := c.prepare(catalog.All(), raw)
	return c, err
}

// TestConfigDefaults checks the defaults of an empty configuration.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestConfigDefaults(t *testing.T) {
	c, err := prepareConfig(map[string]any{"packer_build_name": "hyperv.server 2022"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Profile != "recommended" || c.DryRun || c.Rollback || c.RebootIfRequired {
		t.Errorf("defaults: %+v", c)
	}
	if !c.failOnError() {
		t.Error("fail_on_error must default to true")
	}
	if c.RestartTimeout != 30*time.Minute {
		t.Errorf("restart_timeout default %s", c.RestartTimeout)
	}
	if c.ReportPath != "windows-optimize-report-hyperv.server_2022.json" {
		t.Errorf("report_path default %q", c.ReportPath)
	}
	c, err = prepareConfig(map[string]any{"fail_on_error": false, "restart_timeout": "5m"})
	if err != nil || c.failOnError() || c.RestartTimeout != 5*time.Minute {
		t.Errorf("explicit values: %+v, %v", c, err)
	}
}

// TestConfigOptions checks the conversion to plan options.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestConfigOptions(t *testing.T) {
	c, err := prepareConfig(map[string]any{
		"profile":    "performance",
		"categories": []string{"power", "services"},
		"include":    []string{"storage.short_names_off"},
		"exclude":    []string{"power.high_performance_scheme"},
	})
	if err != nil {
		t.Fatal(err)
	}
	opts := c.options()
	if opts.Profile != catalog.ProfilePerformance || len(opts.Categories) != 2 || opts.Categories[1] != catalog.CategoryServices {
		t.Errorf("options: %+v", opts)
	}
}

// TestConfigRejects checks that invalid configurations fail Prepare with a
// message naming the option.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestConfigRejects(t *testing.T) {
	tests := map[string]struct {
		raw  map[string]any
		want string
	}{
		"unknown profile":      {map[string]any{"profile": "fast"}, "profile:"},
		"custom without list":  {map[string]any{"profile": "custom"}, "custom selects nothing"},
		"unknown category":     {map[string]any{"categories": []string{"network"}}, "categories[0]"},
		"include typo":         {map[string]any{"include": []string{"ux.widget"}}, "include[0]"},
		"exclude typo":         {map[string]any{"exclude": []string{"power.sleep"}}, "exclude[0]"},
		"exclude everything":   {map[string]any{"exclude": []string{"*"}}, "every optimization"},
		"rollback and profile": {map[string]any{"rollback": true, "profile": "vdi"}, "rollback:"},
		"rollback and include": {map[string]any{"rollback": true, "include": []string{"ux.widgets"}}, "rollback:"},
		"short timeout":        {map[string]any{"restart_timeout": "30s"}, "restart_timeout"},
		"unknown option":       {map[string]any{"profiles": "vdi"}, "profiles"},
	}
	for name, tc := range tests {
		_, err := prepareConfig(tc.raw)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want it to mention %q", name, err, tc.want)
		}
	}
}

// TestConfigRollback checks that rollback needs no profile and keeps the
// profile empty in the report.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestConfigRollback(t *testing.T) {
	c, err := prepareConfig(map[string]any{"rollback": true, "dry_run": true})
	if err != nil {
		t.Fatal(err)
	}
	if c.Profile != "" {
		t.Errorf("rollback profile %q", c.Profile)
	}
}

// TestConfigSpec checks that the generated HCL2 spec covers every option.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestConfigSpec(t *testing.T) {
	spec := new(Provisioner).ConfigSpec()
	for _, name := range []string{"profile", "categories", "include", "exclude", "dry_run", "rollback", "fail_on_error", "reboot_if_required", "restart_timeout", "report_path"} {
		if _, ok := spec[name]; !ok {
			t.Errorf("the HCL2 spec has no %s; run go generate ./...", name)
		}
	}
}
