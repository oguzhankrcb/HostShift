package dockere2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultCommandTimeoutMs = 10 * 60 * 1000
	defaultBuildTimeoutMs   = 45 * 60 * 1000
)

type runner struct {
	repoRoot   string
	composeDir string
	stdout     io.Writer
	stderr     io.Writer
}

type matrixPair struct {
	Source      string
	Target      string
	SourceImage string
	TargetImage string
}

type sshConfig struct {
	Aliases    map[string]string
	ConfigPath string
	SSHHome    string
}

type hostshiftCommand struct {
	Command    string
	PrefixArgs []string
	Label      string
}

type commandResult struct {
	Stdout string
	Stderr string
}

type runOptions struct {
	CWD       string
	Env       []string
	Capture   bool
	TimeoutMs int
}

var matrixTargets = map[string][]string{
	"ubuntu22": {"ubuntu22", "ubuntu24", "ubuntu25", "debian12"},
	"debian12": {"ubuntu22", "ubuntu24", "ubuntu25", "debian12", "debian13"},
}

var images = map[string]string{
	"ubuntu22": "ubuntu:22.04",
	"ubuntu24": "ubuntu:24.04",
	"ubuntu25": "ubuntu:25.10",
	"debian12": "debian:12",
	"debian13": "debian:13",
}

var platforms = map[string]string{
	"ubuntu22": "ubuntu:22.04",
	"ubuntu24": "ubuntu:24.04",
	"ubuntu25": "ubuntu:25.10",
	"debian12": "debian:12",
	"debian13": "debian:13",
}

var fixtureConfigFiles = []string{
	"/etc/caddy/Caddyfile",
	"/etc/php/8.3/fpm/php.ini",
	"/etc/php/8.3/fpm/pool.d/fixture.conf",
	"/etc/supervisor/conf.d/fixture-worker.conf",
	"/etc/fail2ban/jail.local",
	"/etc/fail2ban/filter.d/fixture.conf",
	"/etc/memcached.conf",
	"/etc/memcached/conf.d/fixture.conf",
	"/etc/rabbitmq/rabbitmq.conf",
	"/etc/rabbitmq/enabled_plugins",
	"/etc/letsencrypt/live/example.test/fullchain.pem",
	"/etc/letsencrypt/live/example.test/privkey.pem",
	"/etc/logrotate.conf",
	"/etc/logrotate.d/fixture",
}

var fixtureConfigTransferPaths = []string{
	"/etc/caddy",
	"/etc/php/8.3/fpm",
	"/etc/supervisor/conf.d/fixture-worker.conf",
	"/etc/fail2ban",
	"/etc/memcached.conf",
	"/etc/memcached",
	"/etc/rabbitmq",
	"/etc/letsencrypt",
	"/etc/logrotate.conf",
	"/etc/logrotate.d/fixture",
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	r := runner{
		repoRoot:   repoRoot,
		composeDir: filepath.Join(repoRoot, "tests", "integration", "docker"),
		stdout:     stdout,
		stderr:     stderr,
	}
	return r.run(ctx, args)
}

func (r runner) run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("docker-e2e", flag.ContinueOnError)
	fs.SetOutput(r.stderr)
	list := fs.Bool("list", false, "list Docker matrix pairs")
	listImages := fs.Bool("list-images", false, "list Docker fixture base images")
	pullImages := fs.Bool("pull-images", false, "pre-pull Docker fixture base images")
	pairFilter := fs.String("pair", "", "matrix pair filter, for example ubuntu22->debian12")
	if err := fs.Parse(args); err != nil {
		return err
	}

	pairs := dockerMatrixPairs()
	if *pairFilter != "" {
		pairs = filterPairs(pairs, *pairFilter)
		if len(pairs) == 0 {
			return fmt.Errorf("unknown matrix pair: %s", *pairFilter)
		}
	}
	if *list {
		for _, pair := range pairs {
			fmt.Fprintf(r.stdout, "%s -> %s\n", pair.Source, pair.Target)
		}
		return nil
	}
	if *listImages {
		for _, image := range uniqueBaseImages(pairs) {
			fmt.Fprintln(r.stdout, image)
		}
		return nil
	}

	runRealMatrix := os.Getenv("HOSTSHIFT_RUN_DOCKER_MATRIX") == "1"
	commandTimeoutMs, err := readTimeoutMs("HOSTSHIFT_DOCKER_COMMAND_TIMEOUT_MS", defaultCommandTimeoutMs)
	if err != nil {
		return err
	}
	buildTimeoutMs, err := readTimeoutMs("HOSTSHIFT_DOCKER_BUILD_TIMEOUT_MS", defaultBuildTimeoutMs)
	if err != nil {
		return err
	}
	pullTimeoutMs, err := readTimeoutMs("HOSTSHIFT_DOCKER_PULL_TIMEOUT_MS", buildTimeoutMs)
	if err != nil {
		return err
	}
	hostshift := r.resolveHostShiftCommand()

	fmt.Fprintf(r.stdout, "HostShift Docker migration matrix: %d pairs\n", len(pairs))
	fmt.Fprintf(r.stdout, "HostShift CLI: %s\n", hostshift.Label)
	if _, err := r.runCommand(ctx, "docker", []string{"compose", "version"}, runOptions{TimeoutMs: commandTimeoutMs}); err != nil {
		return err
	}
	if *pullImages {
		if err := r.ensureDockerDaemon(ctx, commandTimeoutMs); err != nil {
			return err
		}
		return r.prePullBaseImages(ctx, pairs, commandTimeoutMs, pullTimeoutMs)
	}
	if runRealMatrix {
		if err := r.ensureDockerDaemon(ctx, commandTimeoutMs); err != nil {
			return err
		}
		if os.Getenv("HOSTSHIFT_DOCKER_SKIP_PREPULL") != "1" {
			if err := r.prePullBaseImages(ctx, pairs, commandTimeoutMs, pullTimeoutMs); err != nil {
				return err
			}
		}
	}

	for _, pair := range pairs {
		fmt.Fprintf(r.stdout, "%s -> %s\n", pair.Source, pair.Target)
		if !runRealMatrix {
			continue
		}
		if err := r.runPair(ctx, pair, hostshift, commandTimeoutMs, buildTimeoutMs); err != nil {
			return err
		}
	}
	if !runRealMatrix {
		fmt.Fprintln(r.stdout, "Dry-run only. Set HOSTSHIFT_RUN_DOCKER_MATRIX=1 to execute Docker compose config/build and source immutability checks for each pair.")
	}
	return nil
}

func dockerMatrixPairs() []matrixPair {
	order := []string{"ubuntu22", "debian12"}
	pairs := []matrixPair{}
	for _, source := range order {
		for _, target := range matrixTargets[source] {
			pairs = append(pairs, matrixPair{Source: source, Target: target, SourceImage: images[source], TargetImage: images[target]})
		}
	}
	return pairs
}

func filterPairs(pairs []matrixPair, filter string) []matrixPair {
	out := []matrixPair{}
	for _, pair := range pairs {
		if pair.Source+"->"+pair.Target == filter {
			out = append(out, pair)
		}
	}
	return out
}

func uniqueBaseImages(pairs []matrixPair) []string {
	seen := map[string]bool{}
	for _, pair := range pairs {
		seen[pair.SourceImage] = true
		seen[pair.TargetImage] = true
	}
	out := make([]string, 0, len(seen))
	for image := range seen {
		out = append(out, image)
	}
	sort.Strings(out)
	return out
}

func (r runner) ensureDockerDaemon(ctx context.Context, timeoutMs int) error {
	result, err := r.runCommand(ctx, "docker", []string{"info"}, runOptions{Capture: true, TimeoutMs: timeoutMs})
	if err == nil {
		_ = result
		return nil
	}
	fmt.Fprintln(r.stderr, "Docker daemon is required for HOSTSHIFT_RUN_DOCKER_MATRIX=1.")
	return errors.New("docker daemon unavailable")
}

func (r runner) prePullBaseImages(ctx context.Context, pairs []matrixPair, commandTimeoutMs, pullTimeoutMs int) error {
	for _, image := range uniqueBaseImages(pairs) {
		if r.dockerImageExists(ctx, image, commandTimeoutMs) {
			fmt.Fprintf(r.stdout, "[docker] base image cached: %s\n", image)
			continue
		}
		fmt.Fprintf(r.stdout, "[docker] pulling base image: %s\n", image)
		if _, err := r.runCommand(ctx, "docker", []string{"pull", image}, runOptions{TimeoutMs: pullTimeoutMs}); err != nil {
			return err
		}
	}
	return nil
}

func (r runner) dockerImageExists(ctx context.Context, image string, timeoutMs int) bool {
	_, err := r.runCommand(ctx, "docker", []string{"image", "inspect", image}, runOptions{Capture: true, TimeoutMs: timeoutMs})
	return err == nil
}

func (r runner) runPair(ctx context.Context, pair matrixPair, hostshift hostshiftCommand, commandTimeoutMs, buildTimeoutMs int) error {
	r.logStage(pair, "starting")
	project := sanitizeProject("hostshift-" + pair.Source + "-" + pair.Target)
	sshHome, err := os.MkdirTemp("", project+"-ssh-")
	if err != nil {
		return err
	}
	keyPath := filepath.Join(sshHome, "id_ed25519")
	if err := r.generateKeypair(ctx, keyPath, commandTimeoutMs); err != nil {
		return err
	}
	publicKey, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		return err
	}
	env := append(os.Environ(), "SOURCE_IMAGE="+pair.SourceImage, "TARGET_IMAGE="+pair.TargetImage, "SSH_PUBLIC_KEY="+strings.TrimSpace(string(publicKey)))

	r.logStage(pair, "rendering compose config")
	if _, err := r.runCommand(ctx, "docker", []string{"compose", "-p", project, "-f", "compose.yaml", "config"}, runOptions{CWD: r.composeDir, Env: env, TimeoutMs: commandTimeoutMs}); err != nil {
		return err
	}
	r.logStage(pair, "building fixture images")
	if _, err := r.runCommand(ctx, "docker", dockerComposeBuildArgs(project), runOptions{CWD: r.composeDir, Env: env, TimeoutMs: buildTimeoutMs}); err != nil {
		return err
	}
	defer func() {
		r.logStage(pair, "cleaning up fixtures")
		_, _ = r.runCommand(ctx, "docker", []string{"compose", "-p", project, "-f", "compose.yaml", "down", "--volumes", "--remove-orphans"}, runOptions{CWD: r.composeDir, Env: env, TimeoutMs: commandTimeoutMs})
	}()

	r.logStage(pair, "booting fixtures")
	if _, err := r.runCommand(ctx, "docker", []string{"compose", "-p", project, "-f", "compose.yaml", "up", "-d"}, runOptions{CWD: r.composeDir, Env: env, TimeoutMs: commandTimeoutMs}); err != nil {
		return err
	}
	r.logStage(pair, "verifying source fixture baseline")
	if err := r.verifySourceFixture(ctx, project, env, commandTimeoutMs); err != nil {
		return err
	}
	sourcePort, err := r.lookupPort(ctx, project, "source", commandTimeoutMs)
	if err != nil {
		return err
	}
	targetPort, err := r.lookupPort(ctx, project, "target", commandTimeoutMs)
	if err != nil {
		return err
	}
	sshConfig, err := writeSSHConfig(project, sshHome, keyPath, sourcePort, targetPort)
	if err != nil {
		return err
	}
	r.logStage(pair, "checking SSH connectivity")
	if err := r.verifySSHConnectivity(ctx, sshConfig, commandTimeoutMs); err != nil {
		return err
	}
	serviceSnapshot, err := r.captureSourceServiceSnapshot(ctx, project, env, commandTimeoutMs)
	if err != nil {
		return err
	}
	r.logStage(pair, "running discover")
	if err := r.runHostShiftDiscover(ctx, pair, sshConfig, hostshift, commandTimeoutMs); err != nil {
		return err
	}
	r.logStage(pair, "running dry-run plan/prepare/sync/verify")
	if err := r.runHostShiftDryRuns(ctx, pair, sshConfig, hostshift, commandTimeoutMs); err != nil {
		return err
	}
	r.logStage(pair, "running sync --apply smoke")
	if err := r.runHostShiftSyncApplySmoke(ctx, pair, sshConfig, hostshift, commandTimeoutMs); err != nil {
		return err
	}
	r.logStage(pair, "running verify --apply smoke")
	if err := r.runHostShiftVerifyApplySmoke(ctx, pair, sshConfig, hostshift, commandTimeoutMs); err != nil {
		return err
	}
	r.logStage(pair, "verifying source service immutability")
	if err := r.verifySourceServiceSnapshot(ctx, project, serviceSnapshot, env, commandTimeoutMs); err != nil {
		return err
	}
	r.logStage(pair, "completed successfully")
	return nil
}

func dockerComposeBuildArgs(project string) []string {
	return []string{"compose", "-p", project, "-f", "compose.yaml", "build"}
}

func (r runner) generateKeypair(ctx context.Context, keyPath string, timeoutMs int) error {
	_, err := r.runCommand(ctx, "ssh-keygen", []string{"-q", "-t", "ed25519", "-N", "", "-f", keyPath}, runOptions{TimeoutMs: timeoutMs})
	return err
}

func writeSSHConfig(project, sshHome, keyPath, sourcePort, targetPort string) (sshConfig, error) {
	aliases := map[string]string{"source": project + "-source", "target": project + "-target"}
	sshDir := filepath.Join(sshHome, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return sshConfig{}, err
	}
	configPath := filepath.Join(sshDir, "config")
	config := strings.Join([]string{
		sshHostConfig(aliases["source"], sourcePort, keyPath),
		"",
		sshHostConfig(aliases["target"], targetPort, keyPath),
	}, "\n") + "\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		return sshConfig{}, err
	}
	return sshConfig{Aliases: aliases, ConfigPath: configPath, SSHHome: sshHome}, nil
}

func sshHostConfig(alias, port, keyPath string) string {
	return strings.Join([]string{
		"Host " + alias,
		"  HostName 127.0.0.1",
		"  Port " + port,
		"  User root",
		"  IdentityFile " + keyPath,
		"  BatchMode yes",
		"  IdentitiesOnly yes",
		"  StrictHostKeyChecking no",
		"  UserKnownHostsFile /dev/null",
		"  LogLevel ERROR",
	}, "\n")
}

func (r runner) lookupPort(ctx context.Context, project, service string, timeoutMs int) (string, error) {
	result, err := r.runCommand(ctx, "docker", []string{"compose", "-p", project, "-f", "compose.yaml", "port", service, "2222"}, runOptions{CWD: r.composeDir, Capture: true, TimeoutMs: timeoutMs})
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(result.Stdout)
	parts := strings.Split(value, ":")
	port := parts[len(parts)-1]
	if port == "" || strings.Trim(port, "0123456789") != "" {
		return "", fmt.Errorf("could not parse docker compose port for %s: %s", service, value)
	}
	return port, nil
}

func (r runner) verifySourceFixture(ctx context.Context, project string, env []string, timeoutMs int) error {
	_, err := r.runCommand(ctx, "docker", []string{"compose", "-p", project, "-f", "compose.yaml", "exec", "-T", "source", "sha256sum", "-c", "/fixture/hostshift/source.sha256"}, runOptions{CWD: r.composeDir, Env: env, TimeoutMs: timeoutMs})
	return err
}

const sourceServiceSnapshotScript = `set -eu
while IFS='=' read -r name pid; do
  test -n "${name}"
  test -n "${pid}"
  test -r "/proc/${pid}/stat"
  started="$(awk '{print $22}' "/proc/${pid}/stat")"
  printf '%s=%s:%s\n' "${name}" "${pid}" "${started}"
done < /run/hostshift/source-service-pids`

func (r runner) captureSourceServiceSnapshot(ctx context.Context, project string, env []string, timeoutMs int) (string, error) {
	result, err := r.runCommand(ctx, "docker", []string{"compose", "-p", project, "-f", "compose.yaml", "exec", "-T", "source", "sh", "-lc", sourceServiceSnapshotScript}, runOptions{CWD: r.composeDir, Env: env, Capture: true, TimeoutMs: timeoutMs})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

func (r runner) verifySourceServiceSnapshot(ctx context.Context, project, expected string, env []string, timeoutMs int) error {
	actual, err := r.captureSourceServiceSnapshot(ctx, project, env, timeoutMs)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("source service immutability check failed: before %q; after %q", expected, actual)
	}
	return nil
}

func (r runner) verifySSHConnectivity(ctx context.Context, config sshConfig, timeoutMs int) error {
	env := sshEnv(config)
	if err := r.waitForSSH(ctx, config, config.Aliases["source"], env, timeoutMs); err != nil {
		return err
	}
	return r.waitForSSH(ctx, config, config.Aliases["target"], env, timeoutMs)
}

func (r runner) waitForSSH(ctx context.Context, config sshConfig, alias string, env []string, timeoutMs int) error {
	lastError := ""
	for attempt := 0; attempt < 60; attempt++ {
		_, err := r.runCommand(ctx, "ssh", []string{"-F", config.ConfigPath, alias, "true"}, runOptions{Env: env, Capture: true, TimeoutMs: timeoutMs})
		if err == nil {
			return nil
		}
		lastError = err.Error()
		time.Sleep(time.Second)
	}
	return fmt.Errorf("ssh did not become ready for %s: %s", alias, lastError)
}

func (r runner) runHostShiftDiscover(ctx context.Context, pair matrixPair, config sshConfig, hostshift hostshiftCommand, timeoutMs int) error {
	env := sshEnv(config)
	tempDir, err := os.MkdirTemp("", "hostshift-discover-"+pair.Source+"-"+pair.Target+"-")
	if err != nil {
		return err
	}
	profilePath := filepath.Join(tempDir, "discovered.yaml")
	result, err := r.runHostShift(ctx, hostshift, []string{"discover", "--source", config.Aliases["source"], "--name", pair.Source + "-fixture", "--profile", profilePath, "--json"}, env, timeoutMs)
	if err != nil {
		return err
	}
	var body struct {
		RequiredFailures []string `json:"requiredFailures"`
		Facts            map[string]struct {
			OK bool `json:"ok"`
		} `json:"facts"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &body); err != nil {
		return err
	}
	if len(body.RequiredFailures) != 0 {
		return fmt.Errorf("discover reported required failures for %s: %v", pair.Source, body.RequiredFailures)
	}
	if !body.Facts["osRelease"].OK {
		return fmt.Errorf("discover did not read osRelease for %s", pair.Source)
	}
	discovered, err := os.ReadFile(profilePath)
	if err != nil {
		return err
	}
	if !strings.Contains(string(discovered), "sourcePolicy: strict-read-only") {
		return fmt.Errorf("discover did not write the expected strict source policy profile for %s", pair.Source)
	}
	if !strings.Contains(string(discovered), platforms[pair.Source]) {
		return fmt.Errorf("discover did not capture expected source platform %s", platforms[pair.Source])
	}
	for _, expected := range []string{
		"type: caddy",
		"type: supervisor",
		"type: fail2ban",
		"type: memcached",
		"type: rabbitmq",
		"type: certbot",
		"type: logrotate",
		"name: caddy-config",
		"name: supervisor-config",
		"name: fail2ban-config",
		"name: memcached-config",
		"name: rabbitmq-config",
		"name: letsencrypt",
		"name: logrotate-config",
	} {
		if !strings.Contains(string(discovered), expected) {
			return fmt.Errorf("discover profile for %s missing %q", pair.Source, expected)
		}
	}
	return nil
}

func (r runner) runHostShiftDryRuns(ctx context.Context, pair matrixPair, config sshConfig, hostshift hostshiftCommand, timeoutMs int) error {
	env := sshEnv(config)
	tempDir, err := os.MkdirTemp("", "hostshift-plan-"+pair.Source+"-"+pair.Target+"-")
	if err != nil {
		return err
	}
	profilePath := filepath.Join(tempDir, "matrix-profile.json")
	if err := writeJSON(profilePath, buildMatrixProfile(pair, config.Aliases)); err != nil {
		return err
	}
	plan, err := r.runJSON(ctx, hostshift, []string{"plan", "--profile", profilePath, "--json"}, env, timeoutMs)
	if err != nil {
		return err
	}
	if plan["sourceWillBeModified"] != false {
		return fmt.Errorf("plan must keep source immutable for %s -> %s", pair.Source, pair.Target)
	}
	if err := assertExpectedBlockers(pair, plan["blockers"], "plan"); err != nil {
		return err
	}
	for _, phase := range []string{"prepare", "sync", "verify"} {
		body, err := r.runJSON(ctx, hostshift, []string{phase, "--profile", profilePath, "--json", "--state-dir", tempDir, "--run-id", pair.Source + "-" + pair.Target + "-" + phase}, env, timeoutMs)
		if err != nil {
			return err
		}
		if body["sourceWillBeModified"] != false {
			return fmt.Errorf("%s must keep source immutable for %s -> %s", phase, pair.Source, pair.Target)
		}
		if err := assertExpectedBlockers(pair, body["blockers"], phase); err != nil {
			return err
		}
	}
	return nil
}

func (r runner) runHostShiftSyncApplySmoke(ctx context.Context, pair matrixPair, config sshConfig, hostshift hostshiftCommand, timeoutMs int) error {
	env := sshEnv(config)
	tempDir, err := os.MkdirTemp("", "hostshift-apply-"+pair.Source+"-"+pair.Target+"-")
	if err != nil {
		return err
	}
	profilePath := filepath.Join(tempDir, "matrix-apply-profile.json")
	if err := writeJSON(profilePath, buildApplySmokeProfile(pair, config.Aliases)); err != nil {
		return err
	}
	body, err := r.runJSON(ctx, hostshift, []string{"sync", "--profile", profilePath, "--apply", "--json", "--state-dir", tempDir, "--run-id", pair.Source + "-" + pair.Target + "-sync-apply"}, env, timeoutMs)
	if err != nil {
		return err
	}
	if body["sourceWillBeModified"] != false {
		return fmt.Errorf("sync apply must keep source immutable for %s -> %s", pair.Source, pair.Target)
	}
	results, ok := body["results"].([]any)
	if !ok || len(results) == 0 || !someResultStream(results) {
		return fmt.Errorf("sync apply did not execute the expected stream actions for %s -> %s", pair.Source, pair.Target)
	}
	if err := r.verifyApplyArtifacts(ctx, config, timeoutMs); err != nil {
		return err
	}
	if err := r.verifyMySQLReplication(ctx, config, timeoutMs); err != nil {
		return err
	}
	return r.verifyPostgreSQLReplication(ctx, config, timeoutMs)
}

func (r runner) runHostShiftVerifyApplySmoke(ctx context.Context, pair matrixPair, config sshConfig, hostshift hostshiftCommand, timeoutMs int) error {
	env := sshEnv(config)
	tempDir, err := os.MkdirTemp("", "hostshift-verify-apply-"+pair.Source+"-"+pair.Target+"-")
	if err != nil {
		return err
	}
	profilePath := filepath.Join(tempDir, "matrix-verify-profile.json")
	if err := writeJSON(profilePath, buildVerifySmokeProfile(pair, config.Aliases)); err != nil {
		return err
	}
	body, err := r.runJSON(ctx, hostshift, []string{"verify", "--profile", profilePath, "--apply", "--json", "--state-dir", tempDir, "--run-id", pair.Source + "-" + pair.Target + "-verify-apply"}, env, timeoutMs)
	if err != nil {
		return err
	}
	if body["sourceWillBeModified"] != false {
		return fmt.Errorf("verify apply must keep source immutable for %s -> %s", pair.Source, pair.Target)
	}
	results, ok := body["results"].([]any)
	if !ok || len(results) == 0 || anyDryRunOrSkipped(results) {
		return fmt.Errorf("verify apply did not execute expected target checks for %s -> %s", pair.Source, pair.Target)
	}
	if !hasResultAction(results, "target.check.http.fixture-health") || !hasResultAction(results, "target.check.laravelDatabase.fixture-db") {
		return errors.New("verify apply did not return expected smoke check actions")
	}
	return r.verifyTargetHTTP(ctx, config, timeoutMs)
}

func (r runner) runJSON(ctx context.Context, hostshift hostshiftCommand, args []string, env []string, timeoutMs int) (map[string]any, error) {
	result, err := r.runHostShift(ctx, hostshift, args, env, timeoutMs)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r runner) runHostShift(ctx context.Context, hostshift hostshiftCommand, args []string, env []string, timeoutMs int) (commandResult, error) {
	fullArgs := append([]string{}, hostshift.PrefixArgs...)
	fullArgs = append(fullArgs, args...)
	return r.runCommand(ctx, hostshift.Command, fullArgs, runOptions{CWD: r.repoRoot, Env: env, Capture: true, TimeoutMs: timeoutMs})
}

func (r runner) verifyApplyArtifacts(ctx context.Context, config sshConfig, timeoutMs int) error {
	env := sshEnv(config)
	source := config.Aliases["source"]
	target := config.Aliases["target"]
	items := []struct {
		alias string
		path  string
	}{
		{source, "/fixture/hostshift/source.sha256"},
		{target, "/srv/app/.env"},
		{target, "/srv/app/artisan"},
		{target, "/srv/app/docker-compose.yml"},
		{target, "/srv/app/fixtures/mysql/fixturedb.sql"},
		{target, "/srv/app/fixtures/postgresql/fixturedb.sql"},
		{target, "/srv/app/fixtures/redis/dump.rdb"},
		{target, "/srv/app/fixtures/volume/uploads.tar"},
		{target, "/srv/app/config/standalone.json"},
		{target, "/srv/app/public/index.html"},
		{target, "/etc/nginx/sites-available/example.conf"},
		{target, "/var/lib/redis/dump.rdb"},
		{target, "/srv/hostshift/volumes/uploads/data.txt"},
	}
	for _, path := range fixtureConfigFiles {
		items = append(items, struct {
			alias string
			path  string
		}{target, path})
	}
	for _, item := range items {
		if err := r.verifyRemoteFile(ctx, config, item.alias, item.path, env, timeoutMs); err != nil {
			return err
		}
	}
	comparePaths := []string{"/srv/app/.env", "/srv/app/artisan", "/srv/app/docker-compose.yml", "/srv/app/fixtures/mysql/fixturedb.sql", "/srv/app/fixtures/redis/dump.rdb", "/srv/app/fixtures/volume/uploads.tar", "/etc/nginx/sites-available/example.conf"}
	comparePaths = append(comparePaths, fixtureConfigFiles...)
	for _, remotePath := range comparePaths {
		if err := r.compareRemoteSHA(ctx, config, source, target, remotePath, env, timeoutMs); err != nil {
			return err
		}
	}
	sourceRedis, err := r.remoteSHA(ctx, config, source, "/srv/app/fixtures/redis/dump.rdb", env, timeoutMs)
	if err != nil {
		return err
	}
	targetRedis, err := r.remoteSHA(ctx, config, target, "/var/lib/redis/dump.rdb", env, timeoutMs)
	if err != nil {
		return err
	}
	if sourceRedis != targetRedis {
		return fmt.Errorf("redis snapshot checksum mismatch: %s != %s", sourceRedis, targetRedis)
	}
	sourceVolumeData, err := r.remoteSHA(ctx, config, source, "/srv/app/fixtures/volume/uploads/data.txt", env, timeoutMs)
	if err != nil {
		return err
	}
	targetVolumeData, err := r.remoteSHA(ctx, config, target, "/srv/hostshift/volumes/uploads/data.txt", env, timeoutMs)
	if err != nil {
		return err
	}
	if sourceVolumeData != targetVolumeData {
		return fmt.Errorf("docker volume snapshot checksum mismatch: %s != %s", sourceVolumeData, targetVolumeData)
	}
	_, err = r.runCommand(ctx, "ssh", []string{"-F", config.ConfigPath, source, "sha256sum", "-c", "/fixture/hostshift/source.sha256"}, runOptions{Env: env, TimeoutMs: timeoutMs})
	return err
}

func (r runner) verifyRemoteFile(ctx context.Context, config sshConfig, alias, remotePath string, env []string, timeoutMs int) error {
	_, err := r.runCommand(ctx, "ssh", []string{"-F", config.ConfigPath, alias, "test", "-f", remotePath}, runOptions{Env: env, TimeoutMs: timeoutMs})
	return err
}

func (r runner) compareRemoteSHA(ctx context.Context, config sshConfig, sourceAlias, targetAlias, remotePath string, env []string, timeoutMs int) error {
	source, err := r.remoteSHA(ctx, config, sourceAlias, remotePath, env, timeoutMs)
	if err != nil {
		return err
	}
	target, err := r.remoteSHA(ctx, config, targetAlias, remotePath, env, timeoutMs)
	if err != nil {
		return err
	}
	if source != target {
		return fmt.Errorf("checksum mismatch for %s: %s != %s", remotePath, source, target)
	}
	return nil
}

func (r runner) remoteSHA(ctx context.Context, config sshConfig, alias, remotePath string, env []string, timeoutMs int) (string, error) {
	result, err := r.runCommand(ctx, "ssh", []string{"-F", config.ConfigPath, alias, "sha256sum", remotePath}, runOptions{Env: env, Capture: true, TimeoutMs: timeoutMs})
	if err != nil {
		return "", err
	}
	fields := strings.Fields(strings.TrimSpace(result.Stdout))
	if len(fields) == 0 {
		return "", fmt.Errorf("empty sha256sum output for %s", remotePath)
	}
	return fields[0], nil
}

func (r runner) verifyMySQLReplication(ctx context.Context, config sshConfig, timeoutMs int) error {
	sourceRows, err := r.remoteMySQLScalar(ctx, config, config.Aliases["source"], "fixturedb", "SELECT COUNT(*) FROM pages", timeoutMs)
	if err != nil {
		return err
	}
	targetRows, err := r.remoteMySQLScalar(ctx, config, config.Aliases["target"], "fixturedb", "SELECT COUNT(*) FROM pages", timeoutMs)
	if err != nil {
		return err
	}
	sourceChecksum, err := r.remoteMySQLScalar(ctx, config, config.Aliases["source"], "fixturedb", "CHECKSUM TABLE pages", timeoutMs)
	if err != nil {
		return err
	}
	targetChecksum, err := r.remoteMySQLScalar(ctx, config, config.Aliases["target"], "fixturedb", "CHECKSUM TABLE pages", timeoutMs)
	if err != nil {
		return err
	}
	if sourceRows != "2" {
		return fmt.Errorf("unexpected source mysql row count: %s", sourceRows)
	}
	if targetRows != sourceRows {
		return fmt.Errorf("mysql row count mismatch: %s != %s", sourceRows, targetRows)
	}
	if sourceChecksum != targetChecksum {
		return fmt.Errorf("mysql checksum mismatch: %s != %s", sourceChecksum, targetChecksum)
	}
	return nil
}

func (r runner) verifyPostgreSQLReplication(ctx context.Context, config sshConfig, timeoutMs int) error {
	sourceRows, err := r.remotePostgresScalar(ctx, config, config.Aliases["source"], "fixturepg", "SELECT COUNT(*) FROM metrics", timeoutMs)
	if err != nil {
		return err
	}
	targetRows, err := r.remotePostgresScalar(ctx, config, config.Aliases["target"], "fixturepg", "SELECT COUNT(*) FROM metrics", timeoutMs)
	if err != nil {
		return err
	}
	query := "SELECT md5(string_agg(id::text || ':' || name, ',' ORDER BY id)) FROM metrics"
	sourceChecksum, err := r.remotePostgresScalar(ctx, config, config.Aliases["source"], "fixturepg", query, timeoutMs)
	if err != nil {
		return err
	}
	targetChecksum, err := r.remotePostgresScalar(ctx, config, config.Aliases["target"], "fixturepg", query, timeoutMs)
	if err != nil {
		return err
	}
	if sourceRows != "2" {
		return fmt.Errorf("unexpected source postgresql row count: %s", sourceRows)
	}
	if targetRows != sourceRows {
		return fmt.Errorf("postgresql row count mismatch: %s != %s", sourceRows, targetRows)
	}
	if sourceChecksum != targetChecksum {
		return fmt.Errorf("postgresql checksum mismatch: %s != %s", sourceChecksum, targetChecksum)
	}
	return nil
}

func (r runner) verifyTargetHTTP(ctx context.Context, config sshConfig, timeoutMs int) error {
	env := sshEnv(config)
	result, err := r.runCommand(ctx, "ssh", []string{"-F", config.ConfigPath, config.Aliases["target"], "curl", "--fail", "--silent", "http://127.0.0.1/health"}, runOptions{Env: env, Capture: true, TimeoutMs: timeoutMs})
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.Stdout) != "ok" {
		return fmt.Errorf("unexpected health response: %s", strings.TrimSpace(result.Stdout))
	}
	return nil
}

func (r runner) remoteMySQLScalar(ctx context.Context, config sshConfig, alias, database, query string, timeoutMs int) (string, error) {
	env := sshEnv(config)
	script := "mysql --batch --skip-column-names " + shellQuote(database) + " --execute=" + shellQuote(query)
	result, err := r.runCommand(ctx, "ssh", []string{"-F", config.ConfigPath, alias, "sh -lc " + shellQuote(script)}, runOptions{Env: env, Capture: true, TimeoutMs: timeoutMs})
	if err != nil {
		return "", err
	}
	lines := strings.Fields(strings.TrimSpace(result.Stdout))
	if len(lines) == 0 {
		return "", nil
	}
	return lines[len(lines)-1], nil
}

func (r runner) remotePostgresScalar(ctx context.Context, config sshConfig, alias, database, query string, timeoutMs int) (string, error) {
	env := sshEnv(config)
	script := "psql --username root --dbname " + shellQuote(database) + " --tuples-only --no-align --command " + shellQuote(query)
	result, err := r.runCommand(ctx, "ssh", []string{"-F", config.ConfigPath, alias, "sh -lc " + shellQuote(script)}, runOptions{Env: env, Capture: true, TimeoutMs: timeoutMs})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

func buildMatrixProfile(pair matrixPair, aliases map[string]string) map[string]any {
	fixturePaths := append([]string{"/srv/app", "/etc/nginx/sites-available"}, fixtureConfigTransferPaths...)
	return map[string]any{
		"schemaVersion": 2,
		"name":          "matrix-" + pair.Source + "-to-" + pair.Target,
		"source":        map[string]any{"ssh": aliases["source"]},
		"target":        map[string]any{"ssh": aliases["target"]},
		"sourcePolicy":  "strict-read-only",
		"platforms":     map[string]any{"source": platforms[pair.Source], "target": platforms[pair.Target]},
		"firewall":      map[string]any{"enabled": true, "enable": true, "rules": []map[string]any{{"from": "172.17.0.0/16", "port": 3306, "proto": "tcp"}}},
		"sshd":          map[string]any{"settings": map[string]any{"ClientAliveInterval": 120, "ClientAliveCountMax": 720}},
		"mysql":         map[string]any{"settings": map[string]any{"bindAddress": "0.0.0.0", "mysqlxBindAddress": "127.0.0.1"}},
		"workloads": []map[string]any{
			{"type": "docker-compose", "name": "fixture-compose", "data": map[string]any{"workingDir": "/srv/app", "configFile": "/srv/app/docker-compose.yml"}},
			{"type": "file-set", "name": "fixture-files", "data": map[string]any{"paths": fixturePaths, "targetPath": "/"}},
			{"type": "docker-standalone", "name": "fixture-standalone", "data": map[string]any{"image": "fixture/standalone:latest"}},
			{"type": "caddy", "name": "caddy", "data": map[string]any{"service": "caddy.service", "config": "/etc/caddy/Caddyfile"}},
			{"type": "php-fpm", "name": "php8.3-fpm", "data": map[string]any{"service": "php8.3-fpm.service"}},
			{"type": "supervisor", "name": "supervisor", "data": map[string]any{"service": "supervisor.service"}},
			{"type": "fail2ban", "name": "fail2ban", "data": map[string]any{"service": "fail2ban.service"}},
			{"type": "memcached", "name": "memcached", "data": map[string]any{"service": "memcached.service", "config": "/etc/memcached.conf"}},
			{"type": "rabbitmq", "name": "rabbitmq", "data": map[string]any{"service": "rabbitmq-server.service", "configDir": "/etc/rabbitmq"}},
			{"type": "certbot", "name": "certbot", "data": map[string]any{"configDir": "/etc/letsencrypt"}},
			{"type": "logrotate", "name": "logrotate", "data": map[string]any{"config": "/etc/logrotate.conf"}},
			{"type": "mysql", "name": "fixturedb"},
			{"type": "postgresql", "name": "fixturepg"},
			{"type": "redis", "name": "fixture-cache", "data": map[string]any{"snapshotPath": "/srv/app/fixtures/redis/dump.rdb", "targetPath": "/var/lib/redis/dump.rdb"}},
			{"type": "docker-volume", "name": "uploads", "data": map[string]any{"strategy": "snapshot", "snapshotPath": "/srv/app/fixtures/volume/uploads.tar", "targetPath": "/srv/hostshift/volumes/uploads"}},
		},
		"checks": []map[string]any{
			{"type": "http", "name": "fixture-health", "data": map[string]any{"url": "http://127.0.0.1/health", "timeoutSeconds": 5}},
			{"type": "laravelDatabase", "name": "fixture-db", "data": map[string]any{"container": "fixture-app"}},
		},
		"approved": true,
	}
}

func buildApplySmokeProfile(pair matrixPair, aliases map[string]string) map[string]any {
	fixturePaths := append([]string{"/srv/app", "/etc/nginx/sites-available"}, fixtureConfigTransferPaths...)
	return map[string]any{
		"schemaVersion": 2,
		"name":          "matrix-apply-" + pair.Source + "-to-" + pair.Target,
		"source":        map[string]any{"ssh": aliases["source"]},
		"target":        map[string]any{"ssh": aliases["target"]},
		"sourcePolicy":  "strict-read-only",
		"platforms":     map[string]any{"source": platforms[pair.Source], "target": platforms[pair.Target]},
		"workloads": []map[string]any{
			{"type": "file-set", "name": "fixture-files", "data": map[string]any{"paths": fixturePaths, "targetPath": "/"}},
			{"type": "mysql", "name": "fixturedb"},
			{"type": "postgresql", "name": "fixturepg"},
			{"type": "redis", "name": "fixture-cache", "data": map[string]any{"snapshotPath": "/srv/app/fixtures/redis/dump.rdb", "targetPath": "/var/lib/redis/dump.rdb"}},
			{"type": "docker-volume", "name": "uploads", "data": map[string]any{"strategy": "snapshot", "snapshotPath": "/srv/app/fixtures/volume/uploads.tar", "targetPath": "/srv/hostshift/volumes/uploads"}},
		},
		"approved": true,
	}
}

func buildVerifySmokeProfile(pair matrixPair, aliases map[string]string) map[string]any {
	return map[string]any{
		"schemaVersion": 2,
		"name":          "matrix-verify-" + pair.Source + "-to-" + pair.Target,
		"source":        map[string]any{"ssh": aliases["source"]},
		"target":        map[string]any{"ssh": aliases["target"]},
		"sourcePolicy":  "strict-read-only",
		"platforms":     map[string]any{"source": platforms[pair.Source], "target": platforms[pair.Target]},
		"checks": []map[string]any{
			{"type": "http", "name": "fixture-health", "data": map[string]any{"url": "http://127.0.0.1/health", "timeoutSeconds": 5}},
			{"type": "laravelDatabase", "name": "fixture-db", "data": map[string]any{"container": "fixture-app"}},
		},
		"approved": true,
	}
}

func assertExpectedBlockers(pair matrixPair, raw any, stage string) error {
	blockers, _ := raw.([]any)
	if len(blockers) != 0 {
		return fmt.Errorf("%s unexpectedly blocked for %s -> %s: %v", stage, pair.Source, pair.Target, blockers)
	}
	return nil
}

func someResultStream(results []any) bool {
	for _, item := range results {
		result, _ := item.(map[string]any)
		if result["stream"] == true {
			return true
		}
	}
	return false
}

func anyDryRunOrSkipped(results []any) bool {
	for _, item := range results {
		result, _ := item.(map[string]any)
		if result["dryRun"] == true || result["skipped"] == true {
			return true
		}
	}
	return false
}

func hasResultAction(results []any, actionID string) bool {
	for _, item := range results {
		result, _ := item.(map[string]any)
		if result["actionId"] == actionID && result["error"] == nil {
			return true
		}
	}
	return false
}

func sshEnv(config sshConfig) []string {
	return append(os.Environ(), "HOME="+config.SSHHome, "HOSTSHIFT_SSH_CONFIG="+config.ConfigPath)
}

func (r runner) resolveHostShiftCommand() hostshiftCommand {
	if override := os.Getenv("HOSTSHIFT_DOCKER_HOSTSHIFT_BIN"); override != "" {
		return hostshiftCommand{Command: override, Label: override}
	}
	builtBinary := filepath.Join(r.repoRoot, "dist", "hostshift")
	if info, err := os.Stat(builtBinary); err == nil && !info.IsDir() {
		return hostshiftCommand{Command: builtBinary, Label: builtBinary}
	}
	return hostshiftCommand{Command: "go", PrefixArgs: []string{"run", "./cmd/hostshift"}, Label: "go run ./cmd/hostshift"}
}

func (r runner) runCommand(ctx context.Context, command string, args []string, options runOptions) (commandResult, error) {
	timeout := options.TimeoutMs
	if timeout == 0 {
		timeout = defaultCommandTimeoutMs
	}
	commandCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, command, args...)
	if options.CWD != "" {
		cmd.Dir = options.CWD
	}
	if options.Env != nil {
		cmd.Env = options.Env
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if options.Capture {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	} else {
		cmd.Stdout = r.stdout
		cmd.Stderr = r.stderr
	}
	err := cmd.Run()
	if commandCtx.Err() == context.DeadlineExceeded {
		return commandResult{}, fmt.Errorf("command timed out after %dms: %s", timeout, shellCommand(command, args))
	}
	if err != nil {
		if options.Capture && stderr.String() != "" {
			fmt.Fprint(r.stderr, stderr.String())
		}
		return commandResult{}, fmt.Errorf("command failed: %s", shellCommand(command, args))
	}
	return commandResult{Stdout: stdout.String(), Stderr: stderr.String()}, nil
}

func readTimeoutMs(name string, fallback int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer in milliseconds", name)
	}
	return value, nil
}

func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(cwd, "tests", "integration", "docker", "compose.yaml")); err == nil {
				return cwd, nil
			}
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return "", errors.New("could not locate HostShift repository root")
		}
		cwd = parent
	}
}

func writeJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func sanitizeProject(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('-')
		}
	}
	return builder.String()
}

func shellCommand(command string, args []string) string {
	parts := append([]string{command}, args...)
	for index, part := range parts {
		parts[index] = shellQuote(part)
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (r runner) logStage(pair matrixPair, stage string) {
	fmt.Fprintf(r.stdout, "[%s->%s] %s\n", pair.Source, pair.Target, stage)
}
