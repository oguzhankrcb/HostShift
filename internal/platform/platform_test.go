package platform

import (
	"testing"
	"time"
)

func TestDetectUbuntuAndDebian(t *testing.T) {
	for _, sample := range []struct {
		data string
		id   string
	}{
		{"ID=ubuntu\nVERSION_ID=\"24.04\"\nPRETTY_NAME=\"Ubuntu 24.04 LTS\"\n", "ubuntu"},
		{"ID=debian\nVERSION_ID=\"13\"\nPRETTY_NAME=\"Debian GNU/Linux 13\"\n", "debian"},
	} {
		adapter, err := Detect(ParseOSRelease(sample.data))
		if err != nil {
			t.Fatalf("detect failed: %v", err)
		}
		if adapter.ID() != sample.id {
			t.Fatalf("expected %s, got %s", sample.id, adapter.ID())
		}
	}
}

func TestSupportCatalog(t *testing.T) {
	now := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	ubuntu := Ubuntu{}
	if got := ubuntu.Support(OSRelease{ID: "ubuntu", VersionID: "25.10"}, now); got != SupportInterim {
		t.Fatalf("expected ubuntu 25.10 interim, got %s", got)
	}
	debian := Debian{}
	if got := debian.Support(OSRelease{ID: "debian", VersionID: "12"}, now); got != SupportLTS {
		t.Fatalf("expected debian 12 lts on 2026-06-11, got %s", got)
	}
	if got := debian.Support(OSRelease{ID: "debian", VersionID: "12"}, time.Date(2028, 7, 1, 0, 0, 0, 0, time.UTC)); got != SupportEOL {
		t.Fatalf("expected debian 12 eol after 2028-06-30, got %s", got)
	}
}

func TestAptCapabilityMapping(t *testing.T) {
	for _, adapter := range []Adapter{Ubuntu{}, Debian{}} {
		pkg, ok := adapter.PackageFor("docker-compose")
		if !ok || pkg != "docker-compose-plugin" {
			t.Fatalf("unexpected docker-compose package for %s: %s %v", adapter.ID(), pkg, ok)
		}
		for capability, expected := range map[string]string{"tar": "tar", "curl": "curl"} {
			pkg, ok := adapter.PackageFor(capability)
			if !ok || pkg != expected {
				t.Fatalf("unexpected %s package for %s: %s %v", capability, adapter.ID(), pkg, ok)
			}
		}
		if _, ok := adapter.PackageFor("unknown-capability"); ok {
			t.Fatalf("unexpected unknown capability support for %s", adapter.ID())
		}
	}
	ubuntu := Ubuntu{}
	if pkg, ok := ubuntu.PackageFor("mysql-server"); !ok || pkg != "mysql-server" {
		t.Fatalf("unexpected ubuntu mysql-server mapping: %s %v", pkg, ok)
	}
	debian := Debian{}
	if pkg, ok := debian.PackageFor("mysql-server"); !ok || pkg != "default-mysql-server" {
		t.Fatalf("unexpected debian mysql-server mapping: %s %v", pkg, ok)
	}
	if pkg, ok := debian.PackageFor("mysql-client"); !ok || pkg != "default-mysql-client" {
		t.Fatalf("unexpected debian mysql-client mapping: %s %v", pkg, ok)
	}
}
