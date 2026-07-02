package platform

import "time"

type Debian struct{}

func (Debian) ID() string { return "debian" }

func (Debian) Detect(release OSRelease) bool { return release.ID == "debian" }

func (Debian) Support(release OSRelease, now time.Time) SupportStatus {
	return supportFor(release, now)
}

func (Debian) PackageManager() string { return "apt" }

func (Debian) PackageFor(capability string) (string, bool) {
	switch capability {
	case "mysql-server":
		return "default-mysql-server", true
	case "mysql-client":
		return "default-mysql-client", true
	default:
		return aptPackageFor(capability)
	}
}

func (Debian) FirewallBackends() []string { return []string{"nftables", "ufw"} }

func (Debian) ServiceManager() string { return "systemd" }
