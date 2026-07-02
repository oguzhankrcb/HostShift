package platform

import "time"

type Ubuntu struct{}

func (Ubuntu) ID() string { return "ubuntu" }

func (Ubuntu) Detect(release OSRelease) bool { return release.ID == "ubuntu" }

func (Ubuntu) Support(release OSRelease, now time.Time) SupportStatus {
	return supportFor(release, now)
}

func (Ubuntu) PackageManager() string { return "apt" }

func (Ubuntu) PackageFor(capability string) (string, bool) { return aptPackageFor(capability) }

func (Ubuntu) FirewallBackends() []string { return []string{"ufw", "nftables"} }

func (Ubuntu) ServiceManager() string { return "systemd" }
