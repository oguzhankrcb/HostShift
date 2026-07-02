package platform

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type SupportStatus string

const (
	SupportStandard    SupportStatus = "standard"
	SupportInterim     SupportStatus = "interim"
	SupportLTS         SupportStatus = "lts"
	SupportEOL         SupportStatus = "eol"
	SupportUnsupported SupportStatus = "unsupported"
)

type OSRelease struct {
	ID        string
	VersionID string
	Pretty    string
}

type Adapter interface {
	ID() string
	Detect(OSRelease) bool
	Support(OSRelease, time.Time) SupportStatus
	PackageManager() string
	PackageFor(string) (string, bool)
	FirewallBackends() []string
	ServiceManager() string
}

var aptCapabilities = map[string]string{
	"rsync":             "rsync",
	"tar":               "tar",
	"curl":              "curl",
	"docker-runtime":    "docker.io",
	"docker-compose":    "docker-compose-plugin",
	"nginx":             "nginx",
	"apache":            "apache2",
	"openssh-server":    "openssh-server",
	"mysql-server":      "mysql-server",
	"mysql-client":      "mysql-client",
	"mariadb-client":    "mariadb-client",
	"postgresql-server": "postgresql",
	"postgresql-client": "postgresql-client",
	"redis-tools":       "redis-tools",
	"nftables":          "nftables",
	"ufw":               "ufw",
}

func aptPackageFor(capability string) (string, bool) {
	value, ok := aptCapabilities[capability]
	return value, ok
}

type ReleaseInfo struct {
	ID          string
	VersionID   string
	StandardEOL string
	StatusKind  SupportStatus
}

var catalog = []ReleaseInfo{
	{ID: "ubuntu", VersionID: "22.04", StandardEOL: "2027-05-31", StatusKind: SupportLTS},
	{ID: "ubuntu", VersionID: "24.04", StandardEOL: "2029-05-31", StatusKind: SupportLTS},
	{ID: "ubuntu", VersionID: "25.10", StandardEOL: "2026-07-31", StatusKind: SupportInterim},
	{ID: "ubuntu", VersionID: "26.04", StandardEOL: "2031-05-31", StatusKind: SupportLTS},
	{ID: "debian", VersionID: "12", StandardEOL: "2028-06-30", StatusKind: SupportLTS},
	{ID: "debian", VersionID: "13", StandardEOL: "2028-06-30", StatusKind: SupportStandard},
}

func ParseOSRelease(data string) OSRelease {
	values := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		values[parts[0]] = strings.Trim(parts[1], `"`)
	}
	return OSRelease{
		ID:        values["ID"],
		VersionID: values["VERSION_ID"],
		Pretty:    values["PRETTY_NAME"],
	}
}

func Detect(release OSRelease) (Adapter, error) {
	for _, adapter := range []Adapter{Ubuntu{}, Debian{}} {
		if adapter.Detect(release) {
			return adapter, nil
		}
	}
	return nil, fmt.Errorf("unsupported platform: %s %s", release.ID, release.VersionID)
}

func supportFor(release OSRelease, now time.Time) SupportStatus {
	for _, info := range catalog {
		if info.ID != release.ID || info.VersionID != release.VersionID {
			continue
		}
		eol, err := time.Parse("2006-01-02", info.StandardEOL)
		if err != nil || now.After(eol) {
			return SupportEOL
		}
		return info.StatusKind
	}
	return SupportUnsupported
}

func CompareMajorMinor(a, b string) int {
	parse := func(value string) [2]int {
		parts := strings.Split(value, ".")
		out := [2]int{}
		for i := 0; i < len(parts) && i < 2; i++ {
			n, _ := strconv.Atoi(parts[i])
			out[i] = n
		}
		return out
	}
	pa := parse(a)
	pb := parse(b)
	if pa[0] != pb[0] {
		return pa[0] - pb[0]
	}
	return pa[1] - pb[1]
}
