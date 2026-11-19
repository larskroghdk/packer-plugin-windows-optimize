package catalog

import (
	"slices"
	"strconv"

	"github.com/larskroghdk/packer-plugin-windows-optimize/internal/facts"
)

const (
	SchemeHighPerformance = "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c"
	SubgroupSleep         = "238c9fa8-0aad-41ed-83f4-97be242c8f20"
	SettingStandbyIdle    = "29f6c1db-86da-48c5-9fdb-f2b67b1f44da"
	SettingHibernateIdle  = "9d7815a6-7ee4-497e-8888-515a05f02364"
	SubgroupDisk          = "0012ee47-9041-4b5d-9b77-535fba8b1442"
	SettingDiskIdle       = "6738e2c4-e8a5-4a42-b16a-e040e769756e"
	firstWindows11Build   = 22000
	firstWindows10_2004   = 19041
	lastWindows10Build    = 19045
)

var (
	everyInstallation = []facts.Installation{facts.InstallationClient, facts.InstallationServerDesktop, facts.InstallationServerCore}
	desktop           = []facts.Installation{facts.InstallationClient, facts.InstallationServerDesktop}
	clientOnly        = []facts.Installation{facts.InstallationClient}
	serverDesktop     = []facts.Installation{facts.InstallationServerDesktop}
	enterpriseClient  = []string{
		"enterprise", "enterprisen", "enterprises", "enterprisesn",
		"education", "educationn", "iotenterprise", "iotenterprises", "serverrdsh",
	}
)

var (
	refPowercfg       = Reference{"Powercfg command-line options", "https://learn.microsoft.com/windows-hardware/design/device-experiences/powercfg-command-line-options"}
	refPowerTuning    = Reference{"Power and performance tuning for Windows Server", "https://learn.microsoft.com/windows-server/administration/performance-tuning/hardware/power/power-performance-tuning"}
	refHibernation    = Reference{"How to disable and re-enable hibernation", "https://learn.microsoft.com/troubleshoot/windows-client/setup-upgrade-and-drivers/disable-and-re-enable-hibernation"}
	refServerServices = Reference{"Guidance on disabling system services on Windows Server with Desktop Experience", "https://learn.microsoft.com/windows-server/security/windows-services/security-guidelines-for-disabling-system-services-in-windows-server"}
	refVDI            = Reference{"Optimizing Windows configuration for VDI desktops", "https://learn.microsoft.com/windows-server/remote/remote-desktop-services/remote-desktop-services-vdi-optimize-configuration"}
	refVDOT           = Reference{"Virtual Desktop Optimization Tool (Microsoft VDI team)", "https://github.com/The-Virtual-Desktop-Team/Virtual-Desktop-Optimization-Tool"}
	refCitrix         = Reference{"Citrix Optimizer Tool (CTX224676)", "https://support.citrix.com/external/article/CTX224676/citrix-optimizer-tool.html"}
)

var optimizations = []Optimization{
	{
		ID:             "power.no_idle_sleep",
		Name:           "Never sleep or hibernate when idle",
		Description:    "Sets the sleep and hibernate idle timeouts of the active power scheme to never, on AC and DC power. A virtual machine that sleeps is unreachable: builds stall, monitoring raises false alarms and scheduled work does not run. The Balanced scheme on Windows client sleeps after 30 minutes on AC power by default.",
		Impact:         "None for virtual machines. A physical laptop built from the image would no longer sleep on battery.",
		Category:       CategoryPower,
		Classification: ClassRecommended,
		Profile:        ProfileRecommended,
		Applies:        Applicability{Installations: everyInstallation},
		References:     []Reference{refPowercfg, refPowerTuning},
		Changes: concat(
			powerIndex(SubgroupSleep, SettingStandbyIdle, 0),
			powerIndex(SubgroupSleep, SettingHibernateIdle, 0),
		),
	},
	{
		ID:             "power.hibernation_off",
		Name:           "Turn off hibernation",
		Description:    "Runs the equivalent of powercfg /hibernate off. Windows deletes hiberfil.sys, which is sized at a large share of the guest memory, and Fast Startup, which depends on hibernation, stops caching the kernel session at shutdown. Images get smaller and a shutdown is a real shutdown, which matters before sysprep and for update servicing.",
		Impact:         "Hibernate and Fast Startup are no longer available. Neither is useful in a virtual machine.",
		Category:       CategoryPower,
		Classification: ClassRecommended,
		Profile:        ProfileRecommended,
		Applies:        Applicability{Installations: everyInstallation},
		References:     []Reference{refHibernation, refPowercfg},
		Changes:        []Change{{Target: Target{Kind: KindHibernation}, Want: Value{Present: true, Data: HibernationDisabled}}},
	},
	{
		ID:             "power.high_performance_scheme",
		Name:           "Activate the High performance power scheme",
		Description:    "Makes High performance the active power scheme, which keeps processor performance states and core parking at their maximum. Microsoft recommends it for latency-sensitive workloads. Skipped when the scheme is not installed, which is the case on systems that only expose Balanced.",
		Impact:         "Higher energy use on physical hosts that pass power management through to the guest. Hypervisors usually manage processor power themselves, so the benefit inside a VM is often small.",
		Category:       CategoryPower,
		Classification: ClassOptional,
		Profile:        ProfilePerformance,
		Applies:        Applicability{Installations: everyInstallation},
		References:     []Reference{refPowerTuning, refCitrix},
		Changes:        []Change{{Target: Target{Kind: KindPowerScheme}, Want: Value{Present: true, Data: SchemeHighPerformance}}},
	},
	{
		ID:             "power.no_disk_idle",
		Name:           "Never turn off disks when idle",
		Description:    "Sets the disk idle timeout of the active power scheme to never, on AC and DC power, so the storage stack never powers down a disk and pays the wake-up latency on the next I/O.",
		Impact:         "None for virtual disks. Physical disks would no longer spin down.",
		Category:       CategoryPower,
		Classification: ClassOptional,
		Profile:        ProfilePerformance,
		Applies:        Applicability{Installations: everyInstallation},
		References:     []Reference{refPowercfg, refCitrix},
		Changes:        powerIndex(SubgroupDisk, SettingDiskIdle, 0),
	},
	{
		ID:             "services.xbox_server",
		Name:           "Disable Xbox Live services on Windows Server",
		Description:    "Disables the Xbox Live Auth Manager and Xbox Live Game Save services and the Xbox Live Game Save task. Microsoft's service guidance for Windows Server with Desktop Experience says these services should be disabled.",
		Impact:         "None on a server. The services exist only to support Xbox Live games.",
		Category:       CategoryServices,
		Classification: ClassRecommended,
		Profile:        ProfileRecommended,
		Applies:        Applicability{Installations: serverDesktop},
		References:     []Reference{refServerServices},
		Changes: []Change{
			serviceStart("XblAuthManager", ServiceDisabled),
			serviceStart("XblGameSave", ServiceDisabled),
			taskState(`\Microsoft\Windows\XblGameSave\`, "XblGameSaveTask", TaskDisabled),
		},
	},
	{
		ID:             "services.xbox_client",
		Name:           "Disable Xbox services on Windows client",
		Description:    "Disables the Xbox Live Auth Manager, Xbox Live Game Save, Xbox Live Networking and Xbox Accessory Management services and the Xbox Live Game Save task. Microsoft's VDI guidance disables them on virtual desktops, where Xbox games are not used.",
		Impact:         "The Xbox app, Game Bar and games that use Xbox Live sign-in stop working.",
		Category:       CategoryServices,
		Classification: ClassOptional,
		Profile:        ProfileVDI,
		Applies:        Applicability{Installations: clientOnly},
		References:     []Reference{refVDI, refVDOT},
		Changes: []Change{
			serviceStart("XblAuthManager", ServiceDisabled),
			serviceStart("XblGameSave", ServiceDisabled),
			serviceStart("XboxNetApiSvc", ServiceDisabled),
			serviceStart("XboxGipSvc", ServiceDisabled),
			taskState(`\Microsoft\Windows\XblGameSave\`, "XblGameSaveTask", TaskDisabled),
		},
	},
	{
		ID:             "services.server_consumer",
		Name:           "Disable consumer services on Windows Server",
		Description:    "Disables Downloaded Maps Manager (MapsBroker), Geolocation (lfsvc), Windows Mobile Hotspot (icssvc) and Phone Service (PhoneSvc). Microsoft's service guidance for Windows Server with Desktop Experience marks each as OK to disable. MapsBroker starts automatically (delayed) by default, the others start on trigger.",
		Impact:         "Apps that use downloaded maps, location, mobile hotspot or telephony stop working. None of them is used on a typical server.",
		Category:       CategoryServices,
		Classification: ClassOptional,
		Profile:        ProfilePerformance,
		Applies:        Applicability{Installations: serverDesktop},
		References:     []Reference{refServerServices},
		Changes: []Change{
			serviceStart("MapsBroker", ServiceDisabled),
			serviceStart("lfsvc", ServiceDisabled),
			serviceStart("icssvc", ServiceDisabled),
			serviceStart("PhoneSvc", ServiceDisabled),
		},
	},
	{
		ID:             "services.sysmain",
		Name:           "Disable SysMain (Superfetch)",
		Description:    "Disables the SysMain service, which preloads frequently used applications into memory based on usage history. On non-persistent virtual desktops the history is discarded at every restart, so the service only adds disk I/O and memory pressure.",
		Impact:         "Slower application start on persistent desktops that keep their state across restarts.",
		Category:       CategoryServices,
		Classification: ClassOptional,
		Profile:        ProfileVDI,
		Applies:        Applicability{Installations: clientOnly},
		References:     []Reference{refVDI, refVDOT},
		Changes:        []Change{serviceStart("SysMain", ServiceDisabled)},
	},
	{
		ID:             "tasks.maps",
		Name:           "Disable Maps scheduled tasks",
		Description:    "Disables the Maps toast and Maps update tasks, which download offline map updates and show map notifications.",
		Impact:         "Offline maps are no longer updated automatically.",
		Category:       CategoryScheduledTasks,
		Classification: ClassOptional,
		Profile:        ProfilePerformance,
		Applies:        Applicability{Installations: desktop},
		References:     []Reference{refVDI, refVDOT},
		Changes: []Change{
			taskState(`\Microsoft\Windows\Maps\`, "MapsToastTask", TaskDisabled),
			taskState(`\Microsoft\Windows\Maps\`, "MapsUpdateTask", TaskDisabled),
		},
	},
	{
		ID:             "tasks.winsat",
		Name:           "Disable the WinSAT assessment task",
		Description:    "Disables the Windows System Assessment Tool task, which benchmarks processor, memory, disk and graphics during idle time. The scores describe virtual hardware and are not used by anything in a managed image.",
		Impact:         "The Windows Experience Index is no longer refreshed.",
		Category:       CategoryScheduledTasks,
		Classification: ClassOptional,
		Profile:        ProfilePerformance,
		Applies:        Applicability{Installations: clientOnly},
		References:     []Reference{refVDI, refVDOT},
		Changes:        []Change{taskState(`\Microsoft\Windows\Maintenance\`, "WinSAT", TaskDisabled)},
	},
	{
		ID:             "tasks.power_efficiency_diagnostics",
		Name:           "Disable power efficiency diagnostics",
		Description:    "Disables the AnalyzeSystem task, which traces the system for conditions that cause high energy use. The analysis has no meaning for a virtual machine.",
		Impact:         "No power efficiency reports are produced.",
		Category:       CategoryScheduledTasks,
		Classification: ClassOptional,
		Profile:        ProfilePerformance,
		Applies:        Applicability{Installations: everyInstallation},
		References:     []Reference{refVDI, refVDOT},
		Changes:        []Change{taskState(`\Microsoft\Windows\Power Efficiency Diagnostics\`, "AnalyzeSystem", TaskDisabled)},
	},
	{
		ID:             "tasks.scheduled_defrag",
		Name:           "Disable scheduled drive optimization",
		Description:    "Disables the ScheduledDefrag task. On non-persistent virtual desktops every optimization pass is discarded at restart and only adds I/O on shared storage.",
		Impact:         "Persistent machines lose weekly retrim, which returns freed blocks to thin-provisioned storage. Do not use on persistent or server images.",
		Category:       CategoryScheduledTasks,
		Classification: ClassOptional,
		Profile:        ProfileVDI,
		Applies:        Applicability{Installations: desktop},
		References:     []Reference{refVDI, refCitrix},
		Changes:        []Change{taskState(`\Microsoft\Windows\Defrag\`, "ScheduledDefrag", TaskDisabled)},
	},
	{
		ID:             "tasks.memory_diagnostics",
		Name:           "Disable memory diagnostic tasks",
		Description:    "Disables the tasks that schedule and run the Windows Memory Diagnostic after memory-related events. Virtual machine memory faults are diagnosed on the host, not in the guest.",
		Impact:         "Windows no longer offers a memory test after a memory-related failure.",
		Category:       CategoryScheduledTasks,
		Classification: ClassOptional,
		Profile:        ProfileVDI,
		Applies:        Applicability{Installations: desktop},
		References:     []Reference{refVDI, refVDOT},
		Changes: []Change{
			taskState(`\Microsoft\Windows\MemoryDiagnostic\`, "ProcessMemoryDiagnosticEvents", TaskDisabled),
			taskState(`\Microsoft\Windows\MemoryDiagnostic\`, "RunFullMemoryDiagnostic", TaskDisabled),
		},
	},
	{
		ID:             "telemetry.required_diagnostic_data",
		Name:           "Limit diagnostic data to Required",
		Description:    "Sets the Allow Diagnostic Data policy (AllowTelemetry) to 1, Required. Windows still sends the data it needs to stay secure and up to date, but no longer collects optional diagnostic data, which reduces background collection and upload work.",
		Impact:         "Features that need optional diagnostic data, such as some Windows Insider and analytics scenarios, stop working. Windows Update and Windows Update for Business reports work with Required data.",
		Category:       CategoryTelemetry,
		Classification: ClassOptional,
		Profile:        ProfilePerformance,
		Applies:        Applicability{Installations: everyInstallation},
		References: []Reference{
			{"Policy CSP - System: AllowTelemetry", "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-system#allowtelemetry"},
			{"Configure Windows diagnostic data in your organization", "https://learn.microsoft.com/windows/privacy/configure-windows-diagnostic-data-in-your-organization"},
		},
		Changes: []Change{registryDWORD(`SOFTWARE\Policies\Microsoft\Windows\DataCollection`, "AllowTelemetry", 1)},
	},
	{
		ID:             "telemetry.ceip_tasks",
		Name:           "Disable Customer Experience Improvement Program tasks",
		Description:    "Disables the CEIP Consolidator and USB CEIP tasks, the Autochk proxy and the disk diagnostic data collector. They collect usage data for the retired Customer Experience Improvement Program and run during idle time.",
		Impact:         "None for operation. The data is only used by Microsoft.",
		Category:       CategoryTelemetry,
		Classification: ClassOptional,
		Profile:        ProfilePerformance,
		Applies:        Applicability{Installations: everyInstallation},
		References:     []Reference{refVDI, refVDOT},
		Changes: []Change{
			taskState(`\Microsoft\Windows\Customer Experience Improvement Program\`, "Consolidator", TaskDisabled),
			taskState(`\Microsoft\Windows\Customer Experience Improvement Program\`, "UsbCeip", TaskDisabled),
			taskState(`\Microsoft\Windows\Autochk\`, "Proxy", TaskDisabled),
			taskState(`\Microsoft\Windows\DiskDiagnostic\`, "Microsoft-Windows-DiskDiagnosticDataCollector", TaskDisabled),
		},
	},
	{
		ID:             "telemetry.compatibility_appraiser",
		Name:           "Disable the Compatibility Appraiser tasks",
		Description:    "Disables the Microsoft Compatibility Appraiser tasks, which inventory installed programs and devices for compatibility telemetry. The appraiser is one of the heaviest idle tasks on Windows client.",
		Impact:         "Windows Update uses appraiser data for feature update safeguard holds. Without it a device can be offered a feature update that has a known compatibility issue. Only suitable for images that are replaced instead of upgraded in place.",
		Category:       CategoryTelemetry,
		Classification: ClassHighRisk,
		Profile:        ProfileNone,
		Applies:        Applicability{Installations: clientOnly},
		References:     []Reference{refVDI, refVDOT},
		Changes: []Change{
			taskState(`\Microsoft\Windows\Application Experience\`, "Microsoft Compatibility Appraiser", TaskDisabled),
			taskState(`\Microsoft\Windows\Application Experience\`, "Microsoft Compatibility Appraiser Exp", TaskDisabled),
		},
	},
	{
		ID:             "ux.first_logon_animation",
		Name:           "Turn off the first sign-in animation",
		Description:    "Sets the Show first sign-in animation policy to disabled. New users go straight to the desktop instead of watching the welcome animation while their profile is prepared, which shortens the first logon on every pooled or freshly deployed machine.",
		Impact:         "None. Profile creation still runs, only the animation is skipped.",
		Category:       CategoryUserExperience,
		Classification: ClassRecommended,
		Profile:        ProfileRecommended,
		Applies:        Applicability{Installations: desktop},
		References:     []Reference{{"Policy CSP - WindowsLogon: EnableFirstLogonAnimation", "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-windowslogon#enablefirstlogonanimation"}},
		Changes:        []Change{registryDWORD(`SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`, "EnableFirstLogonAnimation", 0)},
	},
	{
		ID:             "ux.consumer_features",
		Name:           "Turn off Microsoft consumer experiences",
		Description:    "Sets the Turn off Microsoft consumer experiences policy, which stops Windows from installing promoted Store apps and suggestions for every new user. This removes app installs from the first logon and keeps them out of the image. Windows honors the policy on Enterprise, Education and IoT Enterprise editions only.",
		Impact:         "None for managed devices. Users no longer get suggested consumer apps.",
		Category:       CategoryUserExperience,
		Classification: ClassRecommended,
		Profile:        ProfileRecommended,
		Applies:        Applicability{Installations: clientOnly, Editions: enterpriseClient},
		References:     []Reference{{"Policy CSP - Experience: AllowWindowsConsumerFeatures", "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-experience#allowwindowsconsumerfeatures"}},
		Changes:        []Change{registryDWORD(`SOFTWARE\Policies\Microsoft\Windows\CloudContent`, "DisableWindowsConsumerFeatures", 1)},
	},
	{
		ID:             "ux.windows_tips",
		Name:           "Do not show Windows tips",
		Description:    "Sets the Do not show Windows tips policy, which stops the tips and soft-landing notifications that Windows downloads and shows to new users. Windows honors the policy on Enterprise, Education and IoT Enterprise editions only.",
		Impact:         "Users no longer see feature tips.",
		Category:       CategoryUserExperience,
		Classification: ClassOptional,
		Profile:        ProfilePerformance,
		Applies:        Applicability{Installations: clientOnly, Editions: enterpriseClient},
		References:     []Reference{{"Policy CSP - Experience: AllowWindowsTips", "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-experience#allowwindowstips"}},
		Changes:        []Change{registryDWORD(`SOFTWARE\Policies\Microsoft\Windows\CloudContent`, "DisableSoftLanding", 1)},
	},
	{
		ID:             "ux.widgets",
		Name:           "Turn off Widgets",
		Description:    "Sets the Allow widgets policy to disabled on Windows 11. The Widgets board and its WebView-based host process no longer start, which saves memory and network traffic in every session.",
		Impact:         "Users cannot open the Widgets board.",
		Category:       CategoryUserExperience,
		Classification: ClassOptional,
		Profile:        ProfilePerformance,
		Applies:        Applicability{Installations: clientOnly, MinBuild: firstWindows11Build},
		References:     []Reference{{"Policy CSP - NewsAndInterests: AllowNewsAndInterests", "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-newsandinterests#allownewsandinterests"}},
		Changes:        []Change{registryDWORD(`SOFTWARE\Policies\Microsoft\Dsh`, "AllowNewsAndInterests", 0)},
	},
	{
		ID:             "ux.news_and_interests",
		Name:           "Turn off news and interests on the taskbar",
		Description:    "Sets the Enable news and interests on the taskbar policy to disabled on Windows 10, which stops the taskbar feed from loading web content in every session.",
		Impact:         "The taskbar no longer shows weather and news.",
		Category:       CategoryUserExperience,
		Classification: ClassOptional,
		Profile:        ProfilePerformance,
		Applies:        Applicability{Installations: clientOnly, MinBuild: firstWindows10_2004, MaxBuild: lastWindows10Build},
		References:     []Reference{{"Policy CSP - Feeds: FeedsEnabled", "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-feeds#feedsenabled"}},
		Changes:        []Change{registryDWORD(`SOFTWARE\Policies\Microsoft\Windows\Windows Feeds`, "EnableFeeds", 0)},
	},
	{
		ID:             "ux.search_highlights",
		Name:           "Turn off search highlights",
		Description:    "Sets the Allow search highlights policy to disabled, so the search box and search home no longer download and show trending web content.",
		Impact:         "The search home shows no daily highlights. Search itself is unaffected.",
		Category:       CategoryUserExperience,
		Classification: ClassOptional,
		Profile:        ProfilePerformance,
		Applies:        Applicability{Installations: clientOnly, MinBuild: firstWindows10_2004},
		References:     []Reference{{"Policy CSP - Search: AllowSearchHighlights", "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-search#allowsearchhighlights"}},
		Changes:        []Change{registryDWORD(`SOFTWARE\Policies\Microsoft\Windows\Windows Search`, "EnableDynamicContentInWSB", 0)},
	},
	{
		ID:             "ux.server_manager_at_logon",
		Name:           "Do not open Server Manager at logon",
		Description:    "Sets the Do not display Server Manager automatically at logon policy. Server Manager is a heavy console that every administrator logon otherwise starts and that many close immediately.",
		Impact:         "None. Server Manager stays available from the Start menu.",
		Category:       CategoryUserExperience,
		Classification: ClassRecommended,
		Profile:        ProfileRecommended,
		Applies:        Applicability{Installations: serverDesktop},
		References:     []Reference{{"Policy CSP - ADMX_ServerManager: DoNotLaunchServerManager", "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-admx-servermanager#donotlaunchservermanager"}},
		Changes:        []Change{registryDWORD(`SOFTWARE\Policies\Microsoft\Windows\Server\ServerManager`, "DoNotOpenAtLogon", 1)},
	},
	{
		ID:             "ux.background_apps",
		Name:           "Deny Store apps running in the background",
		Description:    "Sets the Let Windows apps run in the background policy to Force Deny. Packaged apps then only run while they are in the foreground, which removes their background work from every session.",
		Impact:         "Packaged apps that rely on background activity, such as notifications, sync and some collaboration and mail clients, stop working correctly while minimized. Test every packaged app the image uses.",
		Category:       CategoryUserExperience,
		Classification: ClassHighRisk,
		Profile:        ProfileNone,
		Applies:        Applicability{Installations: clientOnly},
		References:     []Reference{{"Policy CSP - Privacy: LetAppsRunInBackground", "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-privacy#letappsruninbackground"}},
		Changes:        []Change{registryDWORD(`SOFTWARE\Policies\Microsoft\Windows\AppPrivacy`, "LetAppsRunInBackground", 2)},
	},
	{
		ID:             "storage.short_names_off",
		Name:           "Stop creating 8.3 short file names",
		Description:    "Sets NtfsDisable8dot3NameCreation to 1, so NTFS no longer creates an 8.3 short name next to every long file name on any volume. File creation in large directories gets faster. Existing short names are kept.",
		Impact:         "Legacy applications and installers that address files by short name can fail. Test every application in the image.",
		Category:       CategoryStorage,
		Classification: ClassOptional,
		Profile:        ProfileNone,
		Applies:        Applicability{Installations: everyInstallation},
		RebootRequired: true,
		References: []Reference{
			{"fsutil 8dot3name", "https://learn.microsoft.com/windows-server/administration/windows-commands/fsutil-8dot3name"},
			{"fsutil behavior", "https://learn.microsoft.com/windows-server/administration/windows-commands/fsutil-behavior"},
		},
		Changes: []Change{registryDWORD(`SYSTEM\CurrentControlSet\Control\FileSystem`, "NtfsDisable8dot3NameCreation", 1)},
	},
	{
		ID:             "storage.new_disks_online",
		Name:           "Bring all new disks online automatically",
		Description:    "Sets the partition manager SAN policy to OnlineAll, so disks added to the machine, such as data disks attached after deployment, come online without manual action.",
		Impact:         "Shared disks, for example in a failover cluster, also come online on every node that sees them. Two nodes writing to the same disk corrupts it. Never use on machines that can see shared storage.",
		Category:       CategoryStorage,
		Classification: ClassHighRisk,
		Profile:        ProfileNone,
		Applies:        Applicability{Installations: everyInstallation},
		RebootRequired: true,
		References: []Reference{
			{"Microsoft-Windows-PartitionManager SanPolicy", "https://learn.microsoft.com/windows-hardware/customize/desktop/unattend/microsoft-windows-partitionmanager-sanpolicy"},
			refCitrix,
		},
		Changes: []Change{registryDWORD(`SYSTEM\CurrentControlSet\Services\partmgr\Parameters`, "SanPolicy", 1)},
	},
	{
		ID:             "storage.system_restore_off",
		Name:           "Turn off System Restore",
		Description:    "Sets the Turn off System Restore policy. Windows stops creating restore points, which take Volume Shadow Copy snapshots and disk space. Virtual machines are recovered from hypervisor snapshots or by redeploying the image.",
		Impact:         "Users and support staff cannot roll back driver or update installations with System Restore.",
		Category:       CategoryStorage,
		Classification: ClassOptional,
		Profile:        ProfileVDI,
		Applies:        Applicability{Installations: clientOnly},
		References: []Reference{
			{"Policy CSP - ADMX_SystemRestore: SR_DisableSR", "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-admx-systemrestore#sr_disablesr"},
			refVDI,
		},
		Changes: []Change{registryDWORD(`SOFTWARE\Policies\Microsoft\Windows NT\SystemRestore`, "DisableSR", 1)},
	},
	{
		ID:             "windows_update.exclude_drivers",
		Name:           "Do not include drivers with Windows Update",
		Description:    "Sets the Do not include drivers with Windows Updates policy. A virtual machine gets its drivers from the hypervisor integration tools, and driver updates from Windows Update only add downloads and restarts. Security and quality updates are not affected.",
		Impact:         "Hardware drivers are not updated from Windows Update. Physical machines built from the image need another driver update channel.",
		Category:       CategoryWindowsUpdate,
		Classification: ClassOptional,
		Profile:        ProfileVDI,
		Applies:        Applicability{Installations: everyInstallation},
		References: []Reference{
			{"Policy CSP - Update: ExcludeWUDriversInQualityUpdate", "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-update#excludewudriversinqualityupdate"},
			{"Configure Windows Update client policies", "https://learn.microsoft.com/windows/deployment/update/waas-configure-wufb#exclude-drivers-from-quality-updates"},
		},
		Changes: []Change{registryDWORD(`SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate`, "ExcludeWUDriversInQualityUpdate", 1)},
	},
	{
		ID:             "windows_update.delivery_optimization_http_only",
		Name:           "Use Delivery Optimization without peering",
		Description:    "Sets the Delivery Optimization download mode to HTTP only (0). Updates still download, but virtual desktops cloned from one image no longer cache content for each other and serve it to peers, which costs disk, CPU and network on every clone.",
		Impact:         "No peer-to-peer download savings. Use Microsoft Connected Cache or WSUS if WAN bandwidth matters.",
		Category:       CategoryWindowsUpdate,
		Classification: ClassOptional,
		Profile:        ProfileVDI,
		Applies:        Applicability{Installations: clientOnly},
		References:     []Reference{{"Delivery Optimization reference: download mode", "https://learn.microsoft.com/windows/deployment/do/waas-delivery-optimization-reference#download-mode"}},
		Changes:        []Change{registryDWORD(`SOFTWARE\Policies\Microsoft\Windows\DeliveryOptimization`, "DODownloadMode", 0)},
	},
	{
		ID:             "diagnostics.error_reporting_off",
		Name:           "Turn off Windows Error Reporting",
		Description:    "Sets the Disable Windows Error Reporting policy and disables the QueueReporting task, so crashes are no longer queued, collected and uploaded in the background.",
		Impact:         "Microsoft no longer receives crash reports from the machine, and Windows cannot offer solutions for known problems. Local crash dumps configured with LocalDumps keep working.",
		Category:       CategoryDiagnostics,
		Classification: ClassOptional,
		Profile:        ProfileVDI,
		Applies:        Applicability{Installations: everyInstallation},
		References: []Reference{
			{"WER settings", "https://learn.microsoft.com/windows/win32/wer/wer-settings"},
			refVDI,
		},
		Changes: []Change{
			registryDWORD(`SOFTWARE\Policies\Microsoft\Windows\Windows Error Reporting`, "Disabled", 1),
			taskState(`\Microsoft\Windows\Windows Error Reporting\`, "QueueReporting", TaskDisabled),
		},
	},
	{
		ID:             "diagnostics.shutdown_event_tracker_off",
		Name:           "Turn off the Shutdown Event Tracker",
		Description:    "Sets the Display Shutdown Event Tracker policy to disabled. Windows Server stops asking for a reason at every shutdown and restart, which blocks unattended restarts of session hosts.",
		Impact:         "Shutdowns are still logged (event 1074), but without an administrator-provided reason.",
		Category:       CategoryDiagnostics,
		Classification: ClassOptional,
		Profile:        ProfileVDI,
		Applies:        Applicability{Installations: serverDesktop},
		References:     []Reference{{"Policy CSP - ADMX_Reliability: ShutdownReason", "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-admx-reliability#shutdownreason"}},
		Changes:        []Change{registryDWORD(`SOFTWARE\Policies\Microsoft\Windows NT\Reliability`, "ShutdownReasonOn", 0)},
	},
}

// All returns a deep copy of every optimization in catalog order, so callers
// can never change the embedded catalog.
//
// Author: Lars Krogh
// Version: 1.0.0
func All() []Optimization {
	out := make([]Optimization, len(optimizations))
	for i, o := range optimizations {
		o.Applies.Installations = slices.Clone(o.Applies.Installations)
		o.Applies.Editions = slices.Clone(o.Applies.Editions)
		o.References = slices.Clone(o.References)
		o.Changes = slices.Clone(o.Changes)
		out[i] = o
	}
	return out
}

// registryDWORD returns a change that sets a REG_DWORD below HKLM.
//
// Author: Lars Krogh
// Version: 1.0.0
func registryDWORD(path, name string, data uint32) Change {
	return Change{
		Target: Target{Kind: KindRegistry, Path: path, Name: name},
		Want:   Value{Present: true, Type: RegDWORD, Data: strconv.FormatUint(uint64(data), 10)},
	}
}

// serviceStart returns a change that sets a service's start type.
//
// Author: Lars Krogh
// Version: 1.0.0
func serviceStart(name, start string) Change {
	return Change{Target: Target{Kind: KindService, Name: name}, Want: Value{Present: true, Data: start}}
}

// taskState returns a change that enables or disables a scheduled task.
//
// Author: Lars Krogh
// Version: 1.0.0
func taskState(path, name, state string) Change {
	return Change{Target: Target{Kind: KindScheduledTask, Path: path, Name: name}, Want: Value{Present: true, Data: state}}
}

// powerIndex returns the AC and DC changes that set a power setting to value.
//
// Author: Lars Krogh
// Version: 1.0.0
func powerIndex(subgroup, setting string, value uint32) []Change {
	data := strconv.FormatUint(uint64(value), 10)
	var out []Change
	for _, source := range []string{PowerSourceAC, PowerSourceDC} {
		out = append(out, Change{
			Target: Target{Kind: KindPowerSetting, Subgroup: subgroup, Setting: setting, PowerSource: source},
			Want:   Value{Present: true, Data: data},
		})
	}
	return out
}

// concat joins change lists.
//
// Author: Lars Krogh
// Version: 1.0.0
func concat(lists ...[]Change) []Change {
	var out []Change
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}
