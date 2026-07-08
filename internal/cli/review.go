package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/oguzhankaracabay/hostshift/internal/core"
	"github.com/oguzhankaracabay/hostshift/internal/planner"
	"github.com/oguzhankaracabay/hostshift/internal/profile"
	"github.com/oguzhankaracabay/hostshift/internal/safety"
)

type profileReview struct {
	Profile              string          `json:"profile"`
	Status               string          `json:"status"`
	ReadyForApply        bool            `json:"readyForApply"`
	SourcePolicy         string          `json:"sourcePolicy"`
	SourceWillBeModified bool            `json:"sourceWillBeModified"`
	SafeForAI            bool            `json:"safeForAI"`
	Summary              string          `json:"summary"`
	Findings             []reviewFinding `json:"findings"`
	OperatorChecklist    []string        `json:"operatorChecklist"`
	AIBrief              []string        `json:"aiBrief"`
}

type reviewFinding struct {
	Severity              string `json:"severity"`
	Category              string `json:"category"`
	Message               string `json:"message"`
	Evidence              string `json:"evidence,omitempty"`
	Recommendation        string `json:"recommendation"`
	SuggestedProfilePatch string `json:"suggestedProfilePatch,omitempty"`
}

func review(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	profilePath := fs.String("profile", "", "profile path")
	target := fs.String("target", "", "target ssh alias override")
	jsonOutput := fs.Bool("json", false, "json output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *profilePath == "" {
		return fmt.Errorf("--profile is required")
	}
	prof, err := profile.Load(*profilePath)
	if err != nil {
		return err
	}
	if *target != "" {
		if err := safety.SSHAlias(*target); err != nil {
			return err
		}
		prof.Target.SSH = *target
	}
	plan, err := planner.Build(prof, time.Now().UTC())
	if err != nil {
		return err
	}
	return write(stdout, reviewPlan(prof, plan), *jsonOutput)
}

func reviewPlan(prof profile.Profile, plan planner.Plan) profileReview {
	findings := []reviewFinding{}
	for _, blocker := range plan.Blockers {
		findings = append(findings, reviewFinding{
			Severity:       "blocker",
			Category:       "plan",
			Message:        blocker,
			Recommendation: "Resolve this before any apply command.",
		})
	}
	for _, warning := range plan.Warnings {
		findings = append(findings, reviewFinding{
			Severity:       "warning",
			Category:       "compatibility",
			Message:        warning,
			Recommendation: "Review workload compatibility and add verification checks that prove the target behavior.",
		})
	}
	if prof.SourcePolicy != "strict-read-only" {
		findings = append(findings, reviewFinding{
			Severity:       "blocker",
			Category:       "source-safety",
			Message:        "Source policy is not strict-read-only.",
			Evidence:       prof.SourcePolicy,
			Recommendation: "Keep sourcePolicy set to strict-read-only. HostShift must not mutate the source server.",
		})
	}
	if len(prof.Checks) == 0 {
		findings = append(findings, reviewFinding{
			Severity:       "warning",
			Category:       "verification",
			Message:        "Profile has no verification checks.",
			Recommendation: "Add HTTP, database, service, file, firewall, or nginx checks that prove the migrated workload works on the target.",
		})
	}
	if len(prof.Workloads) == 0 {
		findings = append(findings, reviewFinding{
			Severity:       "warning",
			Category:       "coverage",
			Message:        "Profile has no workloads.",
			Recommendation: "Run discover again or add reviewed workloads before migration.",
		})
	}
	findings = append(findings, workloadReviewFindings(prof)...)
	for _, stream := range plan.Streams {
		if len(stream.Preconditions) == 0 {
			findings = append(findings, reviewFinding{
				Severity:       "info",
				Category:       "stream-review",
				Message:        "Source-to-target stream should be reviewed before apply.",
				Evidence:       stream.ID,
				Recommendation: "Confirm the source command is read-only and the target command writes only to the intended target path or service.",
			})
		}
	}
	for _, action := range plan.Actions {
		if action.HostRole == core.HostRoleTarget && (action.Impact == core.ImpactService || action.Impact == core.ImpactNetwork) {
			findings = append(findings, reviewFinding{
				Severity:       "info",
				Category:       "target-impact",
				Message:        "Plan includes a target-side service or network action.",
				Evidence:       action.ID,
				Recommendation: "Review preconditions and rollback metadata before running the corresponding apply phase.",
			})
		}
	}

	ready := len(plan.Blockers) == 0 && prof.Approved && prof.Target.SSH != ""
	status := "needs-review"
	if len(plan.Blockers) > 0 {
		status = "blocked"
	} else if ready {
		status = "ready-for-dry-run"
	}
	summary := fmt.Sprintf("%s review found %d findings across %d workloads, %d checks, %d actions, and %d streams.", prof.Name, len(findings), len(prof.Workloads), len(prof.Checks), len(plan.Actions), len(plan.Streams))
	checklist := []string{
		"Review every blocker and warning.",
		"Confirm workload list matches the intended source server scope.",
		"Confirm verification checks prove application, database, service, and network behavior on the target.",
		"Run plan, prepare, sync, and verify dry-runs before any apply command.",
		"Run apply phases only from the human-operated CLI after reviewing rollback metadata.",
	}
	brief := []string{
		"Do not run arbitrary source SSH commands.",
		"Do not suggest MCP apply operations; MCP exposes planning, review, and dry-run tools only.",
		"Treat sourceWillBeModified=false as an invariant that must remain true.",
		"Ask for operator approval before any target mutation.",
	}
	if !ready {
		brief = append(brief, "The profile is not ready for apply; explain the blocking or review items before suggesting next commands.")
	} else {
		brief = append(brief, "The profile can proceed to dry-run review; apply still requires a human CLI command.")
	}
	return profileReview{
		Profile:              prof.Name,
		Status:               status,
		ReadyForApply:        ready,
		SourcePolicy:         prof.SourcePolicy,
		SourceWillBeModified: plan.SourceWillBeModified,
		SafeForAI:            !plan.SourceWillBeModified && !strings.Contains(strings.ToLower(status), "apply"),
		Summary:              summary,
		Findings:             findings,
		OperatorChecklist:    checklist,
		AIBrief:              brief,
	}
}

type reviewCheckIndex struct {
	types     map[string]bool
	mysql     map[string]bool
	postgres  map[string]bool
	services  map[string]bool
	filePaths map[string]bool
}

func workloadReviewFindings(prof profile.Profile) []reviewFinding {
	index := buildReviewCheckIndex(prof.Checks)
	findings := []reviewFinding{}
	for _, workload := range prof.Workloads {
		evidence := workload.Type + ":" + workload.Name
		switch workload.Type {
		case "docker-compose", "docker-standalone":
			if !index.types["http"] && !index.types["laravelDatabase"] {
				findings = append(findings, reviewFinding{
					Severity:              "warning",
					Category:              "workload-verification",
					Message:               "Container workload has no HTTP or application database check.",
					Evidence:              evidence,
					Recommendation:        "Add an http check for the public health endpoint or a laravelDatabase check for the migrated application container.",
					SuggestedProfilePatch: suggestedHTTPCheckPatch(workload.Name),
				})
			}
		case "mysql", "mariadb":
			if !index.mysql[workload.Name] {
				findings = append(findings, reviewFinding{
					Severity:              "warning",
					Category:              "workload-verification",
					Message:               "MySQL/MariaDB workload has no scalar data verification check.",
					Evidence:              evidence,
					Recommendation:        "Add a mysqlScalar check that proves an important table count or checksum on the target.",
					SuggestedProfilePatch: suggestedSQLCheckPatch("mysqlScalar", workload.Name),
				})
			}
			findings = append(findings, databaseSecretFindings(workload, "MySQL/MariaDB")...)
		case "postgresql":
			if !index.postgres[workload.Name] {
				findings = append(findings, reviewFinding{
					Severity:              "warning",
					Category:              "workload-verification",
					Message:               "PostgreSQL workload has no scalar data verification check.",
					Evidence:              evidence,
					Recommendation:        "Add a postgresScalar check that proves an important table count or checksum on the target.",
					SuggestedProfilePatch: suggestedSQLCheckPatch("postgresScalar", workload.Name),
				})
			}
			findings = append(findings, databaseSecretFindings(workload, "PostgreSQL")...)
		case "redis":
			if !index.services["redis-server"] && !index.services["redis"] {
				findings = append(findings, reviewFinding{
					Severity:              "info",
					Category:              "workload-verification",
					Message:               "Redis workload has no target serviceActive check.",
					Evidence:              evidence,
					Recommendation:        "Add a serviceActive check for redis-server or redis, plus an application-level check that proves cache/session behavior if relevant.",
					SuggestedProfilePatch: suggestedServiceCheckPatch("redis-server"),
				})
			}
		case "systemd-service":
			service := reviewDataString(workload.Data, "service", "Service")
			if service == "" {
				service = workload.Name
			}
			if !index.services[service] {
				findings = append(findings, reviewFinding{
					Severity:              "warning",
					Category:              "workload-verification",
					Message:               "systemd-service workload has no matching serviceActive check.",
					Evidence:              evidence,
					Recommendation:        "Add a serviceActive check for " + service + " so verify proves the target service is running.",
					SuggestedProfilePatch: suggestedServiceCheckPatch(service),
				})
			}
		case "php-fpm":
			service := reviewDataString(workload.Data, "service", "Service")
			if service == "" {
				service = workload.Name
			}
			if !index.services[service] {
				findings = append(findings, reviewFinding{
					Severity:              "warning",
					Category:              "workload-verification",
					Message:               "PHP-FPM workload has no matching serviceActive check.",
					Evidence:              evidence,
					Recommendation:        "Add a serviceActive check for " + service + " so verify proves the target PHP-FPM pool is running.",
					SuggestedProfilePatch: suggestedServiceCheckPatch(service),
				})
			}
		case "supervisor":
			service := reviewDataString(workload.Data, "service", "Service")
			if service == "" {
				service = "supervisor.service"
			}
			if !index.services[service] {
				findings = append(findings, reviewFinding{
					Severity:              "warning",
					Category:              "workload-verification",
					Message:               "Supervisor workload has no matching serviceActive check.",
					Evidence:              evidence,
					Recommendation:        "Add a serviceActive check for " + service + " so verify proves the target process supervisor is running.",
					SuggestedProfilePatch: suggestedServiceCheckPatch(service),
				})
			}
		case "fail2ban":
			service := reviewDataString(workload.Data, "service", "Service")
			if service == "" {
				service = "fail2ban.service"
			}
			if !index.services[service] && !index.services["fail2ban"] {
				findings = append(findings, reviewFinding{
					Severity:              "warning",
					Category:              "workload-verification",
					Message:               "Fail2ban workload has no matching serviceActive check.",
					Evidence:              evidence,
					Recommendation:        "Add a serviceActive check for " + service + " so verify proves the target intrusion-prevention service is running.",
					SuggestedProfilePatch: suggestedServiceCheckPatch(service),
				})
			}
		case "cron":
			service := reviewDataString(workload.Data, "service", "Service")
			if service == "" {
				if !index.services["cron"] && !index.services["crond"] && !index.services["cron.service"] && !index.services["crond.service"] {
					findings = append(findings, reviewFinding{
						Severity:              "info",
						Category:              "workload-verification",
						Message:               "cron workload has no target serviceActive check.",
						Evidence:              evidence,
						Recommendation:        "Add a serviceActive check for cron or crond when the target platform supports systemd cron verification.",
						SuggestedProfilePatch: suggestedServiceCheckPatch("cron"),
					})
				}
			} else if !index.services[service] {
				findings = append(findings, reviewFinding{
					Severity:              "info",
					Category:              "workload-verification",
					Message:               "cron workload has no matching serviceActive check.",
					Evidence:              evidence,
					Recommendation:        "Add a serviceActive check for " + service + " when the target platform supports systemd cron verification.",
					SuggestedProfilePatch: suggestedServiceCheckPatch(service),
				})
			}
		case "file-set":
			for _, item := range reviewDataStringSlice(workload.Data, "paths", "Paths") {
				if item == "/etc/nginx" || strings.HasPrefix(item, "/etc/nginx/") {
					if !index.types["nginxConfig"] {
						findings = append(findings, reviewFinding{
							Severity:              "warning",
							Category:              "workload-verification",
							Message:               "Nginx file-set has no nginxConfig validation check.",
							Evidence:              evidence,
							Recommendation:        "Add an nginxConfig check so verify tests the target config and reload path.",
							SuggestedProfilePatch: suggestedNginxCheckPatch(),
						})
					}
					break
				}
			}
		case "apache-vhost":
			if !index.services["apache2"] && !index.services["apache2.service"] {
				findings = append(findings, reviewFinding{
					Severity:              "info",
					Category:              "workload-verification",
					Message:               "Apache vhost workload has no target serviceActive check.",
					Evidence:              evidence,
					Recommendation:        "Add a serviceActive check for apache2 after vhost activation if the migrated site depends on Apache.",
					SuggestedProfilePatch: suggestedServiceCheckPatch("apache2"),
				})
			}
		}
	}
	return findings
}

func databaseSecretFindings(workload profile.Workload, label string) []reviewFinding {
	findings := []reviewFinding{}
	for _, key := range []string{"sourcePasswordEnv", "targetPasswordEnv"} {
		if reviewDataString(workload.Data, key) == "" {
			findings = append(findings, reviewFinding{
				Severity:              "info",
				Category:              "secret-review",
				Message:               label + " workload does not declare " + key + ".",
				Evidence:              workload.Type + ":" + workload.Name,
				Recommendation:        "If password authentication is required, reference an environment variable name in " + key + "; never store literal credentials in the profile.",
				SuggestedProfilePatch: suggestedSecretPatch(workload, key),
			})
		}
	}
	return findings
}

func suggestedHTTPCheckPatch(name string) string {
	checkName := safeSnippetName(name, "http")
	return strings.Join([]string{
		"checks:",
		"  - type: http",
		"    name: " + checkName + "-health",
		"    data:",
		"      url: http://127.0.0.1/health",
		"      timeoutSeconds: 10",
	}, "\n")
}

func suggestedSQLCheckPatch(checkType, database string) string {
	checkName := safeSnippetName(database, checkType)
	return strings.Join([]string{
		"checks:",
		"  - type: " + checkType,
		"    name: " + checkName + "-count",
		"    data:",
		"      database: " + database,
		"      query: SELECT COUNT(*) FROM important_table",
		"      expected: \"REPLACE_WITH_EXPECTED_COUNT\"",
	}, "\n")
}

func suggestedServiceCheckPatch(service string) string {
	checkName := safeSnippetName(service, "service")
	return strings.Join([]string{
		"checks:",
		"  - type: serviceActive",
		"    name: " + checkName,
		"    data:",
		"      service: " + service,
	}, "\n")
}

func suggestedNginxCheckPatch() string {
	return strings.Join([]string{
		"checks:",
		"  - type: nginxConfig",
		"    name: nginx-config",
	}, "\n")
}

func suggestedSecretPatch(workload profile.Workload, key string) string {
	return strings.Join([]string{
		"workloads:",
		"  - type: " + workload.Type,
		"    name: " + workload.Name,
		"    data:",
		"      " + key + ": " + suggestedEnvName(workload, key),
	}, "\n")
}

func buildReviewCheckIndex(checks []profile.Check) reviewCheckIndex {
	index := reviewCheckIndex{
		types:     map[string]bool{},
		mysql:     map[string]bool{},
		postgres:  map[string]bool{},
		services:  map[string]bool{},
		filePaths: map[string]bool{},
	}
	for _, check := range checks {
		index.types[check.Type] = true
		switch check.Type {
		case "mysqlScalar":
			index.mysql[reviewDataString(check.Data, "database", "Database")] = true
		case "postgresScalar":
			index.postgres[reviewDataString(check.Data, "database", "Database")] = true
		case "serviceActive":
			index.services[reviewDataString(check.Data, "service", "Service")] = true
		case "fileExists", "fileContains":
			index.filePaths[reviewDataString(check.Data, "path", "Path")] = true
		}
	}
	return index
}

func reviewDataString(data any, keys ...string) string {
	if item, ok := data.(map[string]any); ok {
		for _, key := range keys {
			if raw, ok := item[key]; ok {
				if str, ok := raw.(string); ok {
					return str
				}
			}
		}
	}
	return ""
}

func reviewDataStringSlice(data any, keys ...string) []string {
	if item, ok := data.(map[string]any); ok {
		for _, key := range keys {
			raw, ok := item[key]
			if !ok {
				continue
			}
			switch values := raw.(type) {
			case []any:
				out := []string{}
				for _, value := range values {
					if str, ok := value.(string); ok {
						out = append(out, str)
					}
				}
				return out
			case []string:
				return values
			}
		}
	}
	return nil
}

func safeSnippetName(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	out := strings.Builder{}
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			out.WriteByte('-')
			lastDash = true
		}
	}
	cleaned := strings.Trim(out.String(), "-")
	if cleaned == "" {
		return fallback
	}
	return cleaned
}

func suggestedEnvName(workload profile.Workload, key string) string {
	prefix := "DST"
	if strings.HasPrefix(strings.ToLower(key), "source") {
		prefix = "SRC"
	}
	engine := strings.ToUpper(safeSnippetName(workload.Type, "DB"))
	engine = strings.ReplaceAll(engine, "-", "_")
	name := strings.ToUpper(safeSnippetName(workload.Name, "APP"))
	name = strings.ReplaceAll(name, "-", "_")
	return prefix + "_" + engine + "_" + name + "_PWD"
}
