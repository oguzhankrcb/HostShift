package platform

import "sort"

type PackageCapability struct {
	Name          string `json:"name"`
	UbuntuPackage string `json:"ubuntuPackage"`
	DebianPackage string `json:"debianPackage"`
}

type SupportedRelease struct {
	ID          string        `json:"id"`
	VersionID   string        `json:"versionId"`
	StandardEOL string        `json:"standardEol"`
	StatusKind  SupportStatus `json:"statusKind"`
}

func PackageCapabilities() []PackageCapability {
	names := make([]string, 0, len(aptCapabilities))
	for name := range aptCapabilities {
		names = append(names, name)
	}
	sort.Strings(names)
	ubuntu := Ubuntu{}
	debian := Debian{}
	out := make([]PackageCapability, 0, len(names))
	for _, name := range names {
		ubuntuPackage, _ := ubuntu.PackageFor(name)
		debianPackage, _ := debian.PackageFor(name)
		out = append(out, PackageCapability{
			Name:          name,
			UbuntuPackage: ubuntuPackage,
			DebianPackage: debianPackage,
		})
	}
	return out
}

func SupportedReleases() []SupportedRelease {
	out := make([]SupportedRelease, 0, len(catalog))
	for _, release := range catalog {
		out = append(out, SupportedRelease{
			ID:          release.ID,
			VersionID:   release.VersionID,
			StandardEOL: release.StandardEOL,
			StatusKind:  release.StatusKind,
		})
	}
	return out
}
