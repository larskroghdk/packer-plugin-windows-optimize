package facts

import (
	"fmt"
	"strings"
)

// Installation is the kind of Windows installation an optimization can target.
type Installation string

const (
	InstallationClient        Installation = "client"
	InstallationServerDesktop Installation = "server_desktop"
	InstallationServerCore    Installation = "server_core"
)

const (
	minimumClientBuild = 14393
	minimumServerBuild = 14393
	firstWindows11     = 22000
)

// Release is the classified guest OS. Known is false for a build newer than
// the release tables, which is supported but untested.
type Release struct {
	Name         string
	Installation Installation
	Build        int
	Edition      string
	Known        bool
}

var serverReleases = map[int]string{
	14393: "Windows Server 2016",
	17763: "Windows Server 2019",
	20348: "Windows Server 2022",
	26100: "Windows Server 2025",
}

var clientReleases = map[int]string{
	14393: "Windows 10 version 1607",
	17763: "Windows 10 version 1809",
	19044: "Windows 10 version 21H2",
	19045: "Windows 10 version 22H2",
	22621: "Windows 11 version 22H2",
	22631: "Windows 11 version 23H2",
	26100: "Windows 11 version 24H2",
	26200: "Windows 11 version 25H2",
}

// Classify identifies the release and installation type of o. Builds newer
// than the tables are accepted as unknown releases of the same family, so new
// feature updates keep working with the optimizations whose build range covers
// them.
//
// Returns:
//   - Release: The classified release.
//   - error: The OS is not Windows 10 or later, is a domain controller, is
//     older than the minimum build, or its installation type is unknown.
//
// Author: Lars Krogh
// Version: 1.0.0
func Classify(o OS) (Release, error) {
	if o.Major != 10 {
		return Release{}, fmt.Errorf("unsupported Windows version %d.%d: Windows 10, Windows 11 and Windows Server 2016 or later are required", o.Major, o.Minor)
	}
	r := Release{Build: o.Build, Edition: strings.ToLower(o.EditionID)}
	switch o.ProductType {
	case ProductTypeDomainController:
		return Release{}, fmt.Errorf("domain controllers are not supported: optimizing a domain controller template is out of scope")
	case ProductTypeServer:
		if o.Build < minimumServerBuild {
			return Release{}, fmt.Errorf("unsupported Windows Server build %d: it is older than Windows Server 2016", o.Build)
		}
		switch strings.ToLower(o.InstallationType) {
		case "server":
			r.Installation = InstallationServerDesktop
		case "server core":
			r.Installation = InstallationServerCore
		default:
			return Release{}, fmt.Errorf("unknown Windows Server installation type %q", o.InstallationType)
		}
		r.Name, r.Known = serverReleases[o.Build]
		if !r.Known {
			r.Name = fmt.Sprintf("Windows Server build %d", o.Build)
		}
	case ProductTypeWorkstation:
		if o.Build < minimumClientBuild {
			return Release{}, fmt.Errorf("unsupported Windows client build %d: it is older than Windows 10 version 1607", o.Build)
		}
		if !strings.EqualFold(o.InstallationType, "client") {
			return Release{}, fmt.Errorf("unknown Windows client installation type %q", o.InstallationType)
		}
		r.Installation = InstallationClient
		r.Name, r.Known = clientReleases[o.Build]
		if !r.Known {
			family := "Windows 10"
			if o.Build >= firstWindows11 {
				family = "Windows 11"
			}
			r.Name = fmt.Sprintf("%s build %d", family, o.Build)
		}
	default:
		return Release{}, fmt.Errorf("unknown product type %d", o.ProductType)
	}
	if r.Installation == InstallationServerCore {
		r.Name += " (Server Core)"
	}
	return r, nil
}
