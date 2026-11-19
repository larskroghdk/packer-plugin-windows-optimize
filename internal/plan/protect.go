package plan

import (
	"fmt"
	"strings"

	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/catalog"
	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/facts"
)

var communicatorServices = map[string]string{
	"winrm":        "it is the Packer communicator",
	"http":         "WinRM listens through the HTTP.sys driver",
	"rpcss":        "WinRM, WMI and the service manager depend on it",
	"rpceptmapper": "WinRM, WMI and the service manager depend on it",
	"dcomlaunch":   "WinRM, WMI and the service manager depend on it",
	"winmgmt":      "the guest scripts and many provisioners read system state through WMI",
	"schedule":     "Packer runs elevated commands and restarts through the Task Scheduler",
	"profsvc":      "every WinRM logon loads a user profile",
	"samss":        "local logon, and therefore the WinRM session, depends on it",
	"lsm":          "logon sessions depend on it",
	"keyiso":       "HTTPS listeners use its private key isolation",
	"cryptsvc":     "certificate validation for HTTPS listeners depends on it",
	"bfe":          "the firewall and IPsec depend on it; stopping it blocks network traffic",
	"mpssvc":       "Windows Firewall admits the WinRM connection",
	"nsi":          "the network stack depends on it",
	"tcpip":        "the network stack depends on it",
	"afd":          "the network stack depends on it",
	"dhcp":         "the guest may get its address through DHCP",
	"dnscache":     "name resolution depends on it",
	"nlasvc":       "network location awareness selects the firewall profile",
	"netprofm":     "network location awareness selects the firewall profile",
	"gpsvc":        "Group Policy processing depends on it",
}

var securityServices = map[string]string{
	"windefend":             "Microsoft Defender Antivirus",
	"wdnissvc":              "Microsoft Defender network inspection",
	"sense":                 "Microsoft Defender for Endpoint",
	"securityhealthservice": "Windows Security health reporting",
	"wscsvc":                "Windows Security Center",
	"wuauserv":              "Windows Update",
	"usosvc":                "Windows Update orchestration",
	"waasmedicsvc":          "Windows Update repair",
	"bits":                  "Windows Update downloads",
	"trustedinstaller":      "update and component servicing",
	"eventlog":              "event logging and auditing",
	"diagtrack":             "diagnostic data; its level is managed by policy instead",
}

var protectedRegistryPrefixes = []string{
	`software\policies\microsoft\windows\winrm\`,
	`software\microsoft\windows\currentversion\wsman\`,
	`system\currentcontrolset\services\winrm\`,
	`system\currentcontrolset\services\http\`,
	`software\policies\microsoft\windowsfirewall\`,
	`system\currentcontrolset\services\sharedaccess\`,
	`system\currentcontrolset\services\mpssvc\`,
	`system\currentcontrolset\services\bfe\`,
	`system\currentcontrolset\services\tcpip\`,
	`system\currentcontrolset\services\tcpip6\`,
	`system\currentcontrolset\control\network\`,
	`system\currentcontrolset\control\lsa\`,
	`system\currentcontrolset\control\securityproviders\`,
	`software\policies\microsoft\windows defender\`,
	`software\microsoft\windows defender\`,
	`software\policies\microsoft\windows\windowsupdate\au\`,
	`system\currentcontrolset\services\eventlog\`,
	`software\policies\microsoft\windows\eventlog\`,
}

var protectedRegistryValues = map[string]string{
	`software\microsoft\windows\currentversion\policies\system\enablelua`:                            "remote UAC decides whether the Packer session token is elevated",
	`software\microsoft\windows\currentversion\policies\system\localaccounttokenfilterpolicy`:        "remote UAC decides whether the Packer session token is elevated",
	`software\microsoft\windows\currentversion\policies\system\filteradministratortoken`:             "remote UAC decides whether the Packer session token is elevated",
	`software\policies\microsoft\windows\windowsupdate\disablewindowsupdateaccess`:                   "it turns off Windows Update",
	`software\policies\microsoft\windows\windowsupdate\donotconnecttowindowsupdateinternetlocations`: "it cuts Windows Update off from Microsoft",
}

var protectedTaskPrefixes = []string{
	`\microsoft\windows\windowsupdate\`,
	`\microsoft\windows\updateorchestrator\`,
	`\microsoft\windows\waasmedic\`,
	`\microsoft\windows\windows defender\`,
	`\packer`,
}

// Protect decides whether a change is safe to apply during an image build.
// Static rules protect the Packer communicator, networking, authentication,
// Defender, Windows Update and event logging regardless of the guest. The
// dynamic rule protects every service the guest's WinRM service depends on,
// and fails closed when those dependencies are unknown.
//
// Returns:
//   - string: Why the change must not be applied, or "" when it is safe.
//
// Author: Lars Krogh
// Version: 1.0.0
func Protect(c catalog.Change, f *facts.Facts) string {
	if reason := ProtectStatic(c); reason != "" {
		return reason
	}
	if c.Target.Kind != catalog.KindService {
		return ""
	}
	if f == nil || f.WinRM == nil {
		return "the WinRM service dependencies could not be detected, so no service start type is changed"
	}
	for _, dep := range f.WinRM.Dependencies {
		if strings.EqualFold(dep, c.Target.Name) {
			return fmt.Sprintf("the WinRM service depends on %s on this system", c.Target.Name)
		}
	}
	return ""
}

// ProtectStatic applies the rules that do not depend on the guest. The
// catalog tests require that no catalog change matches one of them.
//
// Returns:
//   - string: Why the change must not be applied, or "" when no rule matches.
//
// Author: Lars Krogh
// Version: 1.0.0
func ProtectStatic(c catalog.Change) string {
	t := c.Target
	switch t.Kind {
	case catalog.KindService:
		name := strings.ToLower(t.Name)
		if why, ok := communicatorServices[name]; ok {
			return fmt.Sprintf("service %s is protected: %s", t.Name, why)
		}
		if what, ok := securityServices[name]; ok {
			return fmt.Sprintf("service %s is protected: it provides %s, which this plugin never weakens", t.Name, what)
		}
	case catalog.KindRegistry:
		key := strings.ToLower(t.Path + `\` + t.Name)
		if why, ok := protectedRegistryValues[key]; ok {
			return fmt.Sprintf(`HKLM\%s\%s is protected: %s`, t.Path, t.Name, why)
		}
		for _, prefix := range protectedRegistryPrefixes {
			if strings.HasPrefix(key, prefix) {
				return fmt.Sprintf(`HKLM\%s is protected: it configures WinRM, networking, authentication, Defender, Windows Update or event logging`, t.Path)
			}
		}
	case catalog.KindScheduledTask:
		path := strings.ToLower(t.Path + t.Name)
		for _, prefix := range protectedTaskPrefixes {
			if strings.HasPrefix(path, prefix) {
				return fmt.Sprintf("task %s%s is protected: it belongs to Windows Update, Defender or Packer", t.Path, t.Name)
			}
		}
	case catalog.KindPowerSetting:
		idle := strings.EqualFold(t.Setting, catalog.SettingStandbyIdle) || strings.EqualFold(t.Setting, catalog.SettingHibernateIdle)
		if idle && c.Want.Data != "0" {
			return "an idle sleep or hibernate timeout would suspend the machine and drop the Packer session"
		}
	}
	return ""
}
