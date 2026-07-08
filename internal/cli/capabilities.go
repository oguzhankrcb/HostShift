package cli

import (
	"flag"
	"io"

	"github.com/oguzhankaracabay/hostshift/internal/platform"
	"github.com/oguzhankaracabay/hostshift/internal/source"
)

type capabilitiesReport struct {
	SourceWillBeModified bool                         `json:"sourceWillBeModified"`
	ApplyToolsExposed    bool                         `json:"applyToolsExposed"`
	SupportedReleases    []platform.SupportedRelease  `json:"supportedReleases"`
	PackageCapabilities  []platform.PackageCapability `json:"packageCapabilities"`
	WorkloadTypes        []capabilityItem             `json:"workloadTypes"`
	CheckTypes           []capabilityItem             `json:"checkTypes"`
	SourceFacts          []string                     `json:"sourceFacts"`
	AIUsage              capabilitiesAIUsage          `json:"aiUsage"`
	Notes                []string                     `json:"notes"`
}

type capabilityItem struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type capabilitiesAIUsage struct {
	PreferredWorkflow []string `json:"preferredWorkflow"`
	SafetyBoundary    []string `json:"safetyBoundary"`
}

func capabilities(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("capabilities", flag.ContinueOnError)
	jsonOutput := fs.Bool("json", false, "json output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return write(stdout, buildCapabilitiesReport(), *jsonOutput)
}

func buildCapabilitiesReport() capabilitiesReport {
	return capabilitiesReport{
		SourceWillBeModified: false,
		ApplyToolsExposed:    false,
		SupportedReleases:    platform.SupportedReleases(),
		PackageCapabilities:  platform.PackageCapabilities(),
		WorkloadTypes: []capabilityItem{
			{Type: "docker-compose", Description: "Validate and cut over Docker Compose projects on the target."},
			{Type: "docker-standalone", Description: "Stream standalone Docker images from source stdout to target image load."},
			{Type: "file-set", Description: "Stream reviewed file or directory paths with tar into the target."},
			{Type: "mysql", Description: "Stream MySQL databases with single-transaction mysqldump into target mysql."},
			{Type: "mariadb", Description: "Stream MariaDB databases with single-transaction dump semantics."},
			{Type: "postgresql", Description: "Stream PostgreSQL custom-format dumps into target pg_restore."},
			{Type: "redis", Description: "Use an existing RDB snapshot or read-only replica stream; source snapshots are never created."},
			{Type: "apache-vhost", Description: "Enable reviewed Apache modules and sites, validate config, and reload target Apache."},
			{Type: "caddy", Description: "Preserve Caddy config, validate target Caddyfile, and reload or restart target Caddy."},
			{Type: "cron", Description: "Preserve cron files and reload target cron service."},
			{Type: "php-fpm", Description: "Preserve PHP-FPM config and reload or restart the target PHP-FPM service."},
			{Type: "supervisor", Description: "Preserve Supervisor config and run target reread/update."},
			{Type: "fail2ban", Description: "Preserve Fail2ban config and reload or restart target Fail2ban."},
			{Type: "memcached", Description: "Preserve Memcached config and restart target Memcached; volatile cache contents are not migrated."},
			{Type: "logrotate", Description: "Preserve Logrotate config and validate target parsing without rotating logs."},
			{Type: "systemd-service", Description: "Enable and start reviewed application systemd units on the target."},
		},
		CheckTypes: []capabilityItem{
			{Type: "http", Description: "Check target HTTP status with optional Host header."},
			{Type: "laravelDatabase", Description: "Run a target-side Laravel-style PDO database connectivity probe."},
			{Type: "fileExists", Description: "Verify a target file path exists."},
			{Type: "fileContains", Description: "Verify a target file contains expected text."},
			{Type: "mysqlScalar", Description: "Run a read-only MySQL scalar query and compare its expected value."},
			{Type: "postgresScalar", Description: "Run a read-only PostgreSQL scalar query and compare its expected value."},
			{Type: "serviceActive", Description: "Verify a target systemd service is active."},
			{Type: "ufwRule", Description: "Verify target UFW rule output contains the expected rule."},
			{Type: "nftRule", Description: "Verify target nftables output contains the expected rule."},
			{Type: "nginxConfig", Description: "Run target nginx configuration validation."},
		},
		SourceFacts: source.FactNames(),
		AIUsage: capabilitiesAIUsage{
			PreferredWorkflow: []string{"doctor", "discover", "plan", "explain", "review", "prepare dry-run", "sync dry-run", "verify dry-run", "cutover dry-run"},
			SafetyBoundary: []string{
				"MCP tools do not expose apply operations.",
				"Source facts are read through allowlisted commands only.",
				"Target mutation requires a reviewed human CLI apply command.",
			},
		},
		Notes: []string{
			"Nginx configuration is modeled as file-set workloads plus nginxConfig checks, not as a standalone nginx workload type.",
			"Unknown platforms and missing package mappings become blockers instead of guessed installs.",
			"Docker named volumes and Redis without an existing snapshot or read-only replica require explicit operator strategy.",
			"Live filesystem streams are not point-in-time snapshots.",
		},
	}
}
