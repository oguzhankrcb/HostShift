package vme2e

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
	"strings"
	"time"
)

const (
	defaultCommandTimeoutMs = 15 * 60 * 1000
	limactlTimeoutMs        = 20 * 60 * 1000
	hostshiftTimeoutMs      = 10 * 60 * 1000
	sshTimeoutMs            = 2 * 60 * 1000
)

type runner struct {
	repoRoot string
	vmDir    string
	stdout   io.Writer
	stderr   io.Writer
}

type matrixConfig struct {
	Providers map[string]json.RawMessage `json:"providers"`
	Platforms map[string]platformConfig  `json:"platforms"`
	Pairs     []matrixPair               `json:"pairs"`
}

type providerConfig struct {
	RequiredBinaries []string `json:"requiredBinaries"`
	BasePackages     []string `json:"basePackages"`
}

type platformConfig struct {
	Family           string `json:"family"`
	Release          string `json:"release"`
	ProviderImageRef string `json:"providerImageRef"`
	TemplateURL      string `json:"templateUrl"`
}

type matrixPair struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type instancePlan struct {
	Provider              string           `json:"provider"`
	InstanceName          string           `json:"instanceName"`
	Role                  string           `json:"role"`
	Platform              platformPlan     `json:"platform"`
	SSH                   sshPlan          `json:"ssh"`
	Mounts                []mountPlan      `json:"mounts"`
	Fixtures              fixturePlan      `json:"fixtures"`
	SourcePolicy          string           `json:"sourcePolicy"`
	Lima                  limaPlan         `json:"lima"`
	CommonBootstrapScript string           `json:"commonBootstrapScript,omitempty"`
	TemplatePath          string           `json:"templatePath,omitempty"`
	Raw                   *json.RawMessage `json:"-"`
}

type platformPlan struct {
	Key              string `json:"key"`
	Family           string `json:"family"`
	Release          string `json:"release"`
	ProviderImageRef string `json:"providerImageRef"`
	TemplateURL      string `json:"templateUrl"`
}

type sshPlan struct {
	Alias     string `json:"alias"`
	User      string `json:"user"`
	LocalPort string `json:"localPort"`
}

type mountPlan struct {
	HostPath  string `json:"hostPath"`
	GuestPath string `json:"guestPath"`
	Writable  bool   `json:"writable"`
}

type fixturePlan struct {
	CommonBootstrapScript string `json:"commonBootstrapScript"`
	RoleBootstrapScript   string `json:"roleBootstrapScript"`
}

type limaPlan struct {
	VMType string `json:"vmType"`
}

type pairWorkspace struct {
	Pair        matrixPair
	Workspace   string
	Provider    string
	SourcePlan  instancePlan
	TargetPlan  instancePlan
	CommandPlan commandPlan
}

type commandPlan struct {
	SourcePolicy string     `json:"sourcePolicy"`
	Commands     [][]string `json:"commands"`
}

type hostshiftCommand struct {
	Command    string
	PrefixArgs []string
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

type sshOptions struct {
	Hostname     string
	Port         string
	User         string
	IdentityFile string
	ProxyCommand string
	ForwardAgent string
}

// Run executes the VM e2e runner. It intentionally mirrors the previous Node
// runner so the public shell entrypoint can move to Go without changing the VM
// workflow semantics.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	r := runner{
		repoRoot: repoRoot,
		vmDir:    filepath.Join(repoRoot, "tests", "e2e", "vm"),
		stdout:   stdout,
		stderr:   stderr,
	}
	return r.run(ctx, args)
}

func (r runner) run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("vm-e2e", flag.ContinueOnError)
	fs.SetOutput(r.stderr)
	list := fs.Bool("list", false, "list VM matrix pairs")
	apply := fs.Bool("apply", false, "execute real VM workflow")
	pairFilter := fs.String("pair", "", "matrix pair filter, for example ubuntu22->debian12")
	providerFlag := fs.String("provider", "", "VM provider")
	emitDir := fs.String("emit-dir", "", "directory for rendered workspaces")
	if err := fs.Parse(args); err != nil {
		return err
	}

	config, err := r.loadMatrix()
	if err != nil {
		return err
	}
	if err := validateMatrix(config); err != nil {
		return err
	}

	providerName := firstNonEmpty(*providerFlag, os.Getenv("HOSTSHIFT_VM_PROVIDER"), config.defaultProvider())
	provider, err := config.provider(providerName)
	if err != nil {
		return err
	}
	pairs := selectPairs(config.Pairs, *pairFilter)
	if *pairFilter != "" && len(pairs) == 0 {
		return fmt.Errorf("unknown matrix pair: %s", *pairFilter)
	}

	if *list {
		for _, pair := range pairs {
			fmt.Fprintf(r.stdout, "%s -> %s\n", pair.Source, pair.Target)
		}
		return nil
	}

	runVM := os.Getenv("HOSTSHIFT_RUN_VM_E2E") == "1"
	fmt.Fprintf(r.stdout, "HostShift VM e2e matrix: %d pairs (provider: %s)\n", len(pairs), providerName)
	if err := r.runProviderPreflight(ctx, providerName, provider); err != nil {
		return err
	}

	workspaces := make([]pairWorkspace, 0, len(pairs))
	for _, pair := range pairs {
		fmt.Fprintf(r.stdout, "%s -> %s\n", pair.Source, pair.Target)
		workspace, err := r.renderPairWorkspace(pair, config.Platforms, providerName, *emitDir)
		if err != nil {
			return err
		}
		fmt.Fprintf(r.stdout, "  rendered workspace: %s\n", workspace.Workspace)
		workspaces = append(workspaces, workspace)
	}

	if !runVM {
		fmt.Fprintln(r.stdout, "Dry-run only. Set HOSTSHIFT_RUN_VM_E2E=1 for provider preflight and VM boot. Add --apply to execute the real provider workflow.")
		return nil
	}

	for _, workspace := range workspaces {
		fmt.Fprintf(r.stdout, "  provider preflight ok: %s -> %s\n", workspace.Pair.Source, workspace.Pair.Target)
	}

	if !*apply {
		fmt.Fprintln(r.stdout, "Provider preflight completed. Add --apply to boot VMs, assemble SSH config, and run HostShift discover/plan dry-runs.")
		return nil
	}

	for _, workspace := range workspaces {
		if err := r.executePair(ctx, workspace); err != nil {
			return err
		}
	}
	fmt.Fprintln(r.stdout, "VM apply executor completed successfully.")
	return nil
}

func (r runner) loadMatrix() (matrixConfig, error) {
	body, err := os.ReadFile(filepath.Join(r.vmDir, "matrix.yaml"))
	if err != nil {
		return matrixConfig{}, err
	}
	var config matrixConfig
	if err := json.Unmarshal(body, &config); err != nil {
		return matrixConfig{}, err
	}
	return config, nil
}

func (c matrixConfig) defaultProvider() string {
	raw, ok := c.Providers["default"]
	if !ok {
		return ""
	}
	var provider string
	_ = json.Unmarshal(raw, &provider)
	return provider
}

func (c matrixConfig) provider(name string) (providerConfig, error) {
	raw, ok := c.Providers[name]
	if !ok {
		return providerConfig{}, fmt.Errorf("unknown VM provider: %s", name)
	}
	var provider providerConfig
	if err := json.Unmarshal(raw, &provider); err != nil {
		return providerConfig{}, fmt.Errorf("invalid VM provider %s: %w", name, err)
	}
	return provider, nil
}

func validateMatrix(config matrixConfig) error {
	if len(config.Providers) == 0 {
		return errors.New("invalid VM matrix: missing providers")
	}
	if len(config.Platforms) == 0 {
		return errors.New("invalid VM matrix: missing platforms")
	}
	if len(config.Pairs) == 0 {
		return errors.New("invalid VM matrix: missing pairs")
	}
	seen := map[string]bool{}
	for _, pair := range config.Pairs {
		source, ok := config.Platforms[pair.Source]
		if !ok {
			return fmt.Errorf("invalid VM matrix: unknown source platform %s", pair.Source)
		}
		target, ok := config.Platforms[pair.Target]
		if !ok {
			return fmt.Errorf("invalid VM matrix: unknown target platform %s", pair.Target)
		}
		if source.TemplateURL == "" {
			return fmt.Errorf("invalid VM matrix: missing templateUrl for %s", pair.Source)
		}
		if target.TemplateURL == "" {
			return fmt.Errorf("invalid VM matrix: missing templateUrl for %s", pair.Target)
		}
		key := pair.Source + "->" + pair.Target
		if seen[key] {
			return fmt.Errorf("invalid VM matrix: duplicate pair %s", key)
		}
		seen[key] = true
	}
	return nil
}

func selectPairs(pairs []matrixPair, filter string) []matrixPair {
	if filter == "" {
		return pairs
	}
	out := []matrixPair{}
	for _, pair := range pairs {
		if pair.Source+"->"+pair.Target == filter {
			out = append(out, pair)
		}
	}
	return out
}

func (r runner) renderPairWorkspace(pair matrixPair, platforms map[string]platformConfig, providerName, emitDir string) (pairWorkspace, error) {
	pairKey := pair.Source + "-to-" + pair.Target
	baseDir := emitDir
	var err error
	if baseDir == "" {
		baseDir, err = os.MkdirTemp("", "hostshift-vm-"+pairKey+"-")
		if err != nil {
			return pairWorkspace{}, err
		}
	} else {
		baseDir, err = filepath.Abs(baseDir)
		if err != nil {
			return pairWorkspace{}, err
		}
	}
	workspaceDir := baseDir
	if emitDir != "" {
		workspaceDir = filepath.Join(baseDir, pairKey)
	}
	fixtureDir := filepath.Join(workspaceDir, "fixtures")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		return pairWorkspace{}, err
	}
	for _, fixture := range []string{"common-bootstrap.sh", "source-bootstrap.sh", "target-bootstrap.sh"} {
		if err := copyFixture(filepath.Join(r.vmDir, "fixtures", fixture), filepath.Join(fixtureDir, fixture)); err != nil {
			return pairWorkspace{}, err
		}
	}

	sourcePlan, err := r.renderProviderPlan(providerName, pairKey, "source", pair.Source, platforms[pair.Source], "./fixtures/source-bootstrap.sh", "strict-read-only")
	if err != nil {
		return pairWorkspace{}, err
	}
	targetPlan, err := r.renderProviderPlan(providerName, pairKey, "target", pair.Target, platforms[pair.Target], "./fixtures/target-bootstrap.sh", "target-mutable")
	if err != nil {
		return pairWorkspace{}, err
	}
	sourcePlan.CommonBootstrapScript = "./fixtures/common-bootstrap.sh"
	targetPlan.CommonBootstrapScript = "./fixtures/common-bootstrap.sh"
	sourcePlan.TemplatePath = "./source.lima.yaml"
	targetPlan.TemplatePath = "./target.lima.yaml"

	if err := os.WriteFile(filepath.Join(workspaceDir, "source.lima.yaml"), []byte(renderLimaTemplate(sourcePlan)), 0o644); err != nil {
		return pairWorkspace{}, err
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "target.lima.yaml"), []byte(renderLimaTemplate(targetPlan)), 0o644); err != nil {
		return pairWorkspace{}, err
	}
	if err := writeJSON(filepath.Join(workspaceDir, "source.plan.json"), sourcePlan); err != nil {
		return pairWorkspace{}, err
	}
	if err := writeJSON(filepath.Join(workspaceDir, "target.plan.json"), targetPlan); err != nil {
		return pairWorkspace{}, err
	}
	pairDocument := map[string]any{"provider": providerName, "sourcePolicy": "strict-read-only", "pair": pair}
	if err := writeJSON(filepath.Join(workspaceDir, "pair.json"), pairDocument); err != nil {
		return pairWorkspace{}, err
	}
	commandPlan := r.buildCommandPlan(workspaceDir, sourcePlan, targetPlan)
	if err := writeJSON(filepath.Join(workspaceDir, "commands.json"), commandPlan); err != nil {
		return pairWorkspace{}, err
	}

	return pairWorkspace{
		Pair:        pair,
		Workspace:   workspaceDir,
		Provider:    providerName,
		SourcePlan:  sourcePlan,
		TargetPlan:  targetPlan,
		CommandPlan: commandPlan,
	}, nil
}

func (r runner) renderProviderPlan(providerName, pairKey, role, platformKey string, platform platformConfig, bootstrapScript, sourcePolicy string) (instancePlan, error) {
	instanceName := sanitizeInstanceName("hostshift-" + pairKey + "-" + role)
	sshAlias := instanceName + "-ssh"
	sshLocalPort := "60023"
	if role == "source" {
		sshLocalPort = "60022"
	}
	templatePath := filepath.Join(r.vmDir, "providers", "lima", "instance-plan.json.tmpl")
	body, err := os.ReadFile(templatePath)
	if err != nil {
		return instancePlan{}, err
	}
	rendered := replaceTemplate(string(body), map[string]string{
		"INSTANCE_NAME":           instanceName,
		"ROLE":                    role,
		"PLATFORM_KEY":            platformKey,
		"PLATFORM_FAMILY":         platform.Family,
		"PLATFORM_RELEASE":        platform.Release,
		"PLATFORM_IMAGE_REF":      platform.ProviderImageRef,
		"PLATFORM_TEMPLATE_URL":   platform.TemplateURL,
		"SSH_ALIAS":               sshAlias,
		"SSH_LOCAL_PORT":          sshLocalPort,
		"REPO_ROOT":               r.repoRoot,
		"COMMON_BOOTSTRAP_SCRIPT": "./fixtures/common-bootstrap.sh",
		"ROLE_BOOTSTRAP_SCRIPT":   bootstrapScript,
		"SOURCE_POLICY":           sourcePolicy,
	})
	var plan instancePlan
	if err := json.Unmarshal([]byte(rendered), &plan); err != nil {
		return instancePlan{}, err
	}
	plan.Provider = providerName
	plan.Lima.VMType = os.Getenv("HOSTSHIFT_VM_LIMA_VM_TYPE")
	return plan, nil
}

func renderLimaTemplate(plan instancePlan) string {
	lines := []string{
		`minimumLimaVersion: "2.0.0"`,
		"base: " + yamlQuote(plan.Platform.TemplateURL),
	}
	if plan.Lima.VMType != "" {
		lines = append(lines, "vmType: "+yamlQuote(plan.Lima.VMType))
	}
	lines = append(lines,
		"mounts:",
		"  - location: "+yamlQuote(firstMountHostPath(plan)),
		"    mountPoint: "+yamlQuote("/mnt/hostshift"),
		"    writable: true",
		"ssh:",
		"  localPort: "+plan.SSH.LocalPort,
		"portForwards:",
		"  - guestIP: 127.0.0.1",
		"    guestPortRange: [1, 65535]",
		"    proto: any",
		"    ignore: true",
		"  - guestIP: 0.0.0.0",
		"    guestIPMustBeZero: false",
		"    guestPortRange: [1, 65535]",
		"    proto: any",
		"    ignore: true",
		"provision:",
		"  - mode: system",
		"    file:",
		"      url: "+yamlQuote(plan.CommonBootstrapScript),
		"  - mode: system",
		"    file:",
		"      url: "+yamlQuote(plan.Fixtures.RoleBootstrapScript),
		"",
	)
	return strings.Join(lines, "\n")
}

func firstMountHostPath(plan instancePlan) string {
	if len(plan.Mounts) == 0 {
		return ""
	}
	return plan.Mounts[0].HostPath
}

func (r runner) buildCommandPlan(workspaceDir string, sourcePlan, targetPlan instancePlan) commandPlan {
	template := func(filename string) string { return filepath.Join(workspaceDir, filename) }
	discoveredProfile := filepath.Join(workspaceDir, "discovered.profile.yaml")
	fixtureProfile := filepath.Join(workspaceDir, "fixture.profile.json")
	stateDir := filepath.Join(workspaceDir, "state")
	hostshift := r.resolveHostShiftCommand()
	hostshiftCommand := func(args ...string) []string {
		out := []string{hostshift.Command}
		out = append(out, hostshift.PrefixArgs...)
		out = append(out, args...)
		return out
	}
	return commandPlan{
		SourcePolicy: "strict-read-only",
		Commands: [][]string{
			{"limactl", "validate", template("source.lima.yaml")},
			{"limactl", "validate", template("target.lima.yaml")},
			{"limactl", "start", "--tty=false", "--name", sourcePlan.InstanceName, template("source.lima.yaml")},
			{"limactl", "start", "--tty=false", "--name", targetPlan.InstanceName, template("target.lima.yaml")},
			{"limactl", "show-ssh", "--format=options", sourcePlan.InstanceName},
			{"limactl", "show-ssh", "--format=options", targetPlan.InstanceName},
			hostshiftCommand("discover", "--source", sourcePlan.SSH.Alias, "--name", sourcePlan.Platform.Key, "--profile", discoveredProfile, "--json"),
			hostshiftCommand("plan", "--profile", discoveredProfile, "--target", targetPlan.SSH.Alias, "--json"),
			hostshiftCommand("plan", "--profile", fixtureProfile, "--target", targetPlan.SSH.Alias, "--json"),
			hostshiftCommand("prepare", "--profile", fixtureProfile, "--target", targetPlan.SSH.Alias, "--apply", "--json", "--state-dir", stateDir, "--run-id", "vm-prepare"),
			hostshiftCommand("sync", "--profile", fixtureProfile, "--target", targetPlan.SSH.Alias, "--apply", "--json", "--state-dir", stateDir, "--run-id", "vm-sync"),
			hostshiftCommand("cutover", "--profile", fixtureProfile, "--target", targetPlan.SSH.Alias, "--json", "--state-dir", stateDir, "--run-id", "vm-cutover-preview"),
			hostshiftCommand("cutover", "--profile", fixtureProfile, "--target", targetPlan.SSH.Alias, "--apply", "--confirm", "<confirmationCode>", "--json", "--state-dir", stateDir, "--run-id", "vm-cutover"),
			hostshiftCommand("verify", "--profile", fixtureProfile, "--target", targetPlan.SSH.Alias, "--apply", "--json", "--state-dir", stateDir, "--run-id", "vm-verify"),
			{"limactl", "stop", targetPlan.InstanceName},
			{"limactl", "start", targetPlan.InstanceName},
			{"limactl", "show-ssh", "--format=options", sourcePlan.InstanceName},
			{"limactl", "show-ssh", "--format=options", targetPlan.InstanceName},
			hostshiftCommand("verify", "--profile", fixtureProfile, "--target", targetPlan.SSH.Alias, "--apply", "--json", "--state-dir", stateDir, "--run-id", "vm-post-reboot-verify"),
			{"limactl", "stop", targetPlan.InstanceName},
			{"limactl", "stop", sourcePlan.InstanceName},
			{"limactl", "delete", "--force", targetPlan.InstanceName},
			{"limactl", "delete", "--force", sourcePlan.InstanceName},
		},
	}
}

func (r runner) executePair(ctx context.Context, workspace pairWorkspace) error {
	instances := []struct {
		filename string
		plan     instancePlan
	}{
		{"source.lima.yaml", workspace.SourcePlan},
		{"target.lima.yaml", workspace.TargetPlan},
	}
	keepInstances := os.Getenv("HOSTSHIFT_VM_KEEP_INSTANCES") == "1"
	stateDir := filepath.Join(workspace.Workspace, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}

	defer func() {
		if keepInstances {
			return
		}
		for index := len(instances) - 1; index >= 0; index-- {
			instance := instances[index]
			_, _ = r.runCommand(ctx, "limactl", []string{"stop", instance.plan.InstanceName}, runOptions{CWD: workspace.Workspace, Capture: true, TimeoutMs: limactlTimeoutMs})
			_, _ = r.runCommand(ctx, "limactl", []string{"delete", "--force", instance.plan.InstanceName}, runOptions{CWD: workspace.Workspace, Capture: true, TimeoutMs: limactlTimeoutMs})
		}
	}()

	for _, instance := range instances {
		templatePath := filepath.Join(workspace.Workspace, instance.filename)
		r.logStep(workspace, "validating "+instance.plan.Role+" template")
		if _, err := r.runCommand(ctx, "limactl", []string{"validate", templatePath}, runOptions{CWD: workspace.Workspace, TimeoutMs: limactlTimeoutMs}); err != nil {
			return err
		}
	}
	for _, instance := range instances {
		templatePath := filepath.Join(workspace.Workspace, instance.filename)
		r.logStep(workspace, "starting "+instance.plan.Role+" VM")
		if _, err := r.runCommand(ctx, "limactl", []string{"start", "--tty=false", "--name", instance.plan.InstanceName, templatePath}, runOptions{CWD: workspace.Workspace, TimeoutMs: limactlTimeoutMs}); err != nil {
			return err
		}
	}

	r.logStep(workspace, "assembling SSH config")
	sshConfig, err := r.buildApplySSHConfig(ctx, workspace)
	if err != nil {
		return err
	}
	sshConfigPath := filepath.Join(workspace.Workspace, "ssh_config")
	if err := os.WriteFile(sshConfigPath, []byte(sshConfig), 0o600); err != nil {
		return err
	}

	r.logStep(workspace, "capturing source snapshot before migration")
	before, err := r.captureSourceSnapshot(ctx, workspace.SourcePlan.SSH.Alias, sshConfigPath)
	if err != nil {
		return err
	}
	if err := r.runHostShiftWorkflow(ctx, workspace, sshConfigPath, stateDir); err != nil {
		return err
	}
	if err := r.verifyTargetBootPersistence(ctx, workspace, sshConfigPath, stateDir); err != nil {
		return err
	}
	r.logStep(workspace, "capturing source snapshot after migration")
	after, err := r.captureSourceSnapshot(ctx, workspace.SourcePlan.SSH.Alias, sshConfigPath)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("source immutability check failed for %s -> %s", workspace.Pair.Source, workspace.Pair.Target)
	}
	return nil
}

func (r runner) buildApplySSHConfig(ctx context.Context, workspace pairWorkspace) (string, error) {
	source, err := r.readLimaSSHOptions(ctx, workspace.SourcePlan.InstanceName)
	if err != nil {
		return "", err
	}
	target, err := r.readLimaSSHOptions(ctx, workspace.TargetPlan.InstanceName)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		sshAliasConfig(workspace.SourcePlan.SSH.Alias, source),
		"",
		sshAliasConfig(workspace.TargetPlan.SSH.Alias, target),
	}, "\n"), nil
}

func (r runner) readLimaSSHOptions(ctx context.Context, instanceName string) (sshOptions, error) {
	result, err := r.runCommand(ctx, "limactl", []string{"show-ssh", "--format=options", instanceName}, runOptions{Capture: true, TimeoutMs: limactlTimeoutMs})
	if err != nil {
		return sshOptions{}, err
	}
	values := map[string]string{}
	for _, rawLine := range strings.Split(result.Stdout, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			return sshOptions{}, fmt.Errorf("unexpected limactl show-ssh output: %s", line)
		}
		values[key] = stripWrappedQuotes(value)
	}
	for _, key := range []string{"Hostname", "Port", "User", "IdentityFile"} {
		if values[key] == "" {
			return sshOptions{}, fmt.Errorf("missing %s from limactl show-ssh for %s", key, instanceName)
		}
	}
	return sshOptions{
		Hostname:     values["Hostname"],
		Port:         values["Port"],
		User:         values["User"],
		IdentityFile: values["IdentityFile"],
		ProxyCommand: values["ProxyCommand"],
		ForwardAgent: values["ForwardAgent"],
	}, nil
}

func sshAliasConfig(alias string, options sshOptions) string {
	lines := []string{
		"Host " + alias,
		"  HostName " + options.Hostname,
		"  Port " + options.Port,
		"  User " + options.User,
		"  IdentityFile " + options.IdentityFile,
		"  BatchMode yes",
		"  IdentitiesOnly yes",
		"  StrictHostKeyChecking no",
		"  UserKnownHostsFile /dev/null",
		"  LogLevel ERROR",
	}
	if options.ProxyCommand != "" {
		lines = append(lines, "  ProxyCommand "+options.ProxyCommand)
	}
	if options.ForwardAgent != "" {
		lines = append(lines, "  ForwardAgent "+options.ForwardAgent)
	}
	return strings.Join(lines, "\n")
}

func (r runner) runHostShiftWorkflow(ctx context.Context, workspace pairWorkspace, sshConfigPath, stateDir string) error {
	discoveredProfile := filepath.Join(workspace.Workspace, "discovered.profile.yaml")
	fixtureProfile := filepath.Join(workspace.Workspace, "fixture.profile.json")
	env := append(os.Environ(), "HOSTSHIFT_SSH_CONFIG="+sshConfigPath, "HOSTSHIFT_TARGET_SUDO=1")

	r.logStep(workspace, "running hostshift discover")
	discover, err := r.runHostShift(ctx, []string{"discover", "--source", workspace.SourcePlan.SSH.Alias, "--name", workspace.Pair.Source + "-fixture", "--profile", discoveredProfile, "--json"}, env)
	if err != nil {
		return err
	}
	var discoverBody struct {
		RequiredFailures []string `json:"requiredFailures"`
	}
	if err := json.Unmarshal([]byte(discover.Stdout), &discoverBody); err != nil {
		return err
	}
	if len(discoverBody.RequiredFailures) != 0 {
		return fmt.Errorf("discover reported required failures for %s: %v", workspace.Pair.Source, discoverBody.RequiredFailures)
	}

	r.logStep(workspace, "planning discovered profile")
	discoveredPlan, err := r.runHostShift(ctx, []string{"plan", "--profile", discoveredProfile, "--target", workspace.TargetPlan.SSH.Alias, "--json"}, env)
	if err != nil {
		return err
	}
	if err := assertDiscoveryPlanReviewGate(discoveredPlan.Stdout); err != nil {
		return err
	}

	if err := writeJSON(fixtureProfile, buildFixtureProfile(workspace)); err != nil {
		return err
	}

	r.logStep(workspace, "planning fixture profile")
	plan, err := r.runHostShift(ctx, []string{"plan", "--profile", fixtureProfile, "--target", workspace.TargetPlan.SSH.Alias, "--json"}, env)
	if err != nil {
		return err
	}
	if err := assertPlanKeepsSourceImmutable(plan.Stdout, "fixture plan"); err != nil {
		return err
	}

	runPrefix := workspace.Pair.Source + "-" + workspace.Pair.Target
	phases := []struct {
		name         string
		runID        string
		expectStream bool
	}{
		{"prepare", runPrefix + "-prepare", false},
		{"sync", runPrefix + "-sync", true},
	}
	for _, phase := range phases {
		r.logStep(workspace, "running hostshift "+phase.name+" --apply")
		result, err := r.runHostShift(ctx, []string{phase.name, "--profile", fixtureProfile, "--target", workspace.TargetPlan.SSH.Alias, "--apply", "--json", "--state-dir", stateDir, "--run-id", phase.runID}, env)
		if err != nil {
			return err
		}
		if err := assertPhaseResult(result.Stdout, phase.name, phase.expectStream); err != nil {
			return err
		}
	}

	r.logStep(workspace, "previewing hostshift cutover")
	cutoverPreview, err := r.runHostShift(ctx, []string{"cutover", "--profile", fixtureProfile, "--target", workspace.TargetPlan.SSH.Alias, "--json", "--state-dir", stateDir, "--run-id", runPrefix + "-cutover-preview"}, env)
	if err != nil {
		return err
	}
	if err := assertPlanKeepsSourceImmutable(cutoverPreview.Stdout, "cutover preview"); err != nil {
		return err
	}
	confirmationCode, err := cutoverConfirmationCode(cutoverPreview.Stdout)
	if err != nil {
		return err
	}
	r.logStep(workspace, "running hostshift cutover --apply")
	cutover, err := r.runHostShift(ctx, []string{"cutover", "--profile", fixtureProfile, "--target", workspace.TargetPlan.SSH.Alias, "--apply", "--confirm", confirmationCode, "--json", "--state-dir", stateDir, "--run-id", runPrefix + "-cutover"}, env)
	if err != nil {
		return err
	}
	if err := assertPhaseResult(cutover.Stdout, "cutover", false); err != nil {
		return err
	}

	r.logStep(workspace, "running hostshift verify --apply")
	verify, err := r.runHostShift(ctx, []string{"verify", "--profile", fixtureProfile, "--target", workspace.TargetPlan.SSH.Alias, "--apply", "--json", "--state-dir", stateDir, "--run-id", runPrefix + "-verify"}, env)
	if err != nil {
		return err
	}
	if err := assertPhaseResult(verify.Stdout, "verify", false); err != nil {
		return err
	}
	return nil
}

func cutoverConfirmationCode(raw string) (string, error) {
	var body struct {
		ConfirmationCode string `json:"confirmationCode"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return "", err
	}
	if body.ConfirmationCode == "" {
		return "", fmt.Errorf("cutover preview returned no confirmation code")
	}
	return body.ConfirmationCode, nil
}

func (r runner) verifyTargetBootPersistence(ctx context.Context, workspace pairWorkspace, sshConfigPath, stateDir string) error {
	fixtureProfile := filepath.Join(workspace.Workspace, "fixture.profile.json")
	runPrefix := workspace.Pair.Source + "-" + workspace.Pair.Target
	r.logStep(workspace, "restarting target VM for boot persistence")
	if _, err := r.runCommand(ctx, "limactl", []string{"stop", workspace.TargetPlan.InstanceName}, runOptions{CWD: workspace.Workspace, TimeoutMs: limactlTimeoutMs}); err != nil {
		return err
	}
	if _, err := r.runCommand(ctx, "limactl", []string{"start", workspace.TargetPlan.InstanceName}, runOptions{CWD: workspace.Workspace, TimeoutMs: limactlTimeoutMs}); err != nil {
		return err
	}
	sshConfig, err := r.buildApplySSHConfig(ctx, workspace)
	if err != nil {
		return err
	}
	if err := os.WriteFile(sshConfigPath, []byte(sshConfig), 0o600); err != nil {
		return err
	}
	env := append(os.Environ(), "HOSTSHIFT_SSH_CONFIG="+sshConfigPath, "HOSTSHIFT_TARGET_SUDO=1")
	r.logStep(workspace, "running post-reboot hostshift verify --apply")
	result, err := r.runHostShift(ctx, []string{"verify", "--profile", fixtureProfile, "--target", workspace.TargetPlan.SSH.Alias, "--apply", "--json", "--state-dir", stateDir, "--run-id", runPrefix + "-post-reboot-verify"}, env)
	if err != nil {
		return err
	}
	return assertPhaseResult(result.Stdout, "post-reboot verify", false)
}

func buildFixtureProfile(workspace pairWorkspace) map[string]any {
	return map[string]any{
		"schemaVersion": 2,
		"name":          "vm-" + workspace.Pair.Source + "-to-" + workspace.Pair.Target,
		"source":        map[string]any{"ssh": workspace.SourcePlan.SSH.Alias},
		"target":        map[string]any{"ssh": workspace.TargetPlan.SSH.Alias},
		"sourcePolicy":  "strict-read-only",
		"platforms": map[string]any{
			"source": workspace.SourcePlan.Platform.Family + ":" + workspace.SourcePlan.Platform.Release,
			"target": workspace.TargetPlan.Platform.Family + ":" + workspace.TargetPlan.Platform.Release,
		},
		"firewall": map[string]any{
			"enabled": true,
			"enable":  false,
			"rules":   []map[string]any{{"from": "172.17.0.0/16", "port": 3306, "proto": "tcp"}},
		},
		"sshd":  map[string]any{"settings": map[string]any{"ClientAliveInterval": 120, "ClientAliveCountMax": 720}},
		"mysql": map[string]any{"settings": map[string]any{"bindAddress": "0.0.0.0", "mysqlxBindAddress": "127.0.0.1"}},
		"workloads": []map[string]any{
			{"type": "file-set", "name": "vm-fixture-files", "data": map[string]any{"paths": []string{"/srv/hostshift-fixture", "/etc/nginx/sites-available/hostshift-fixture.conf", "/etc/nginx/sites-enabled/hostshift-fixture.conf", "/etc/apache2/ports.conf", "/etc/apache2/sites-available/hostshift-fixture.conf", "/etc/systemd/system/hostshift-fixture-app.service"}, "targetPath": "/"}},
			{"type": "mysql", "name": "hostshiftvm"},
			{"type": "postgresql", "name": "hostshiftpg"},
			{"type": "apache-vhost", "name": "hostshift-fixture", "data": map[string]any{"sites": []string{"hostshift-fixture.conf"}}},
			{"type": "systemd-service", "name": "hostshift-fixture-app", "data": map[string]any{"service": "hostshift-fixture-app.service", "unitPath": "/etc/systemd/system/hostshift-fixture-app.service"}},
		},
		"checks": []map[string]any{
			{"type": "nginxConfig", "name": "reload-nginx"},
			{"type": "serviceActive", "name": "ssh-service", "data": map[string]any{"service": "ssh"}},
			{"type": "serviceActive", "name": "nginx-service", "data": map[string]any{"service": "nginx"}},
			{"type": "serviceActive", "name": "apache-service", "data": map[string]any{"service": "apache2"}},
			{"type": "serviceActive", "name": "fixture-app-service", "data": map[string]any{"service": "hostshift-fixture-app.service"}},
			{"type": "serviceActive", "name": "mysql-service", "data": map[string]any{"service": "mysql"}},
			{"type": "serviceActive", "name": "postgres-service", "data": map[string]any{"service": "postgresql"}},
			{"type": "ufwRule", "name": "mysql-firewall-rule", "data": map[string]any{"from": "172.17.0.0/16", "port": 3306, "proto": "tcp"}},
			{"type": "nftRule", "name": "mysql-nft-rule", "data": map[string]any{"family": "inet", "table": "hostshift", "chain": "input", "contains": "tcp dport 3306 accept"}},
			{"type": "fileExists", "name": "health-file", "data": map[string]any{"path": "/srv/hostshift-fixture/public/health"}},
			{"type": "fileExists", "name": "nginx-site", "data": map[string]any{"path": "/etc/nginx/sites-available/hostshift-fixture.conf"}},
			{"type": "fileExists", "name": "apache-site", "data": map[string]any{"path": "/etc/apache2/sites-available/hostshift-fixture.conf"}},
			{"type": "fileExists", "name": "fixture-app-unit", "data": map[string]any{"path": "/etc/systemd/system/hostshift-fixture-app.service"}},
			{"type": "fileContains", "name": "health-content", "data": map[string]any{"path": "/srv/hostshift-fixture/public/health", "contains": "ok"}},
			{"type": "fileContains", "name": "sshd-keepalive", "data": map[string]any{"path": "/etc/ssh/sshd_config.d/99-hostshift.conf", "contains": "ClientAliveInterval 120"}},
			{"type": "fileContains", "name": "mysql-bind", "data": map[string]any{"path": "/etc/mysql/mysql.conf.d/99-hostshift-bind.cnf", "contains": "bind-address = 0.0.0.0"}},
			{"type": "http", "name": "health-http", "data": map[string]any{"url": "http://127.0.0.1/health", "timeoutSeconds": 10}},
			{"type": "http", "name": "apache-health-http", "data": map[string]any{"url": "http://127.0.0.1:8080/health", "timeoutSeconds": 10}},
			{"type": "mysqlScalar", "name": "mysql-row-count", "data": map[string]any{"database": "hostshiftvm", "query": "SELECT COUNT(*) FROM pages", "expected": "2"}},
			{"type": "mysqlScalar", "name": "mysql-checksum", "data": map[string]any{"database": "hostshiftvm", "query": "SELECT MD5(GROUP_CONCAT(CONCAT(id, ':', slug, ':', body) ORDER BY id SEPARATOR ',')) FROM pages", "expected": "b56d589972734ead12a0069c3ebb4178"}},
			{"type": "postgresScalar", "name": "postgres-row-count", "data": map[string]any{"database": "hostshiftpg", "query": "SELECT COUNT(*) FROM metrics", "expected": "2"}},
			{"type": "postgresScalar", "name": "postgres-checksum", "data": map[string]any{"database": "hostshiftpg", "query": "SELECT md5(string_agg(id::text || ':' || name, ',' ORDER BY id)) FROM metrics", "expected": "e5926976ef869d2387a6e12b8bcc0cdd"}},
		},
		"approved": true,
	}
}

func assertPlanKeepsSourceImmutable(raw, phase string) error {
	var body struct {
		SourceWillBeModified bool     `json:"sourceWillBeModified"`
		Blockers             []string `json:"blockers"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return err
	}
	if body.SourceWillBeModified {
		return fmt.Errorf("%s must keep source immutable", phase)
	}
	if len(body.Blockers) > 0 {
		return fmt.Errorf("%s reported blockers: %v", phase, body.Blockers)
	}
	return nil
}

func assertDiscoveryPlanReviewGate(raw string) error {
	var body struct {
		SourceWillBeModified bool     `json:"sourceWillBeModified"`
		Blockers             []string `json:"blockers"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return err
	}
	if body.SourceWillBeModified {
		return fmt.Errorf("discovered plan must keep source immutable")
	}
	wanted := map[string]bool{
		"Profile is not approved": true,
		"Target platform is unknown; package capabilities could not be mapped to distribution packages": true,
	}
	for _, blocker := range body.Blockers {
		delete(wanted, blocker)
	}
	if len(wanted) != 0 {
		return fmt.Errorf("discovered plan did not preserve review gates: missing %v; blockers %v", wanted, body.Blockers)
	}
	return nil
}

func assertPhaseResult(raw, phase string, expectStream bool) error {
	var body struct {
		SourceWillBeModified bool     `json:"sourceWillBeModified"`
		Blockers             []string `json:"blockers"`
		Results              []struct {
			ActionID string `json:"actionId"`
			Error    string `json:"error"`
			DryRun   bool   `json:"dryRun"`
			Skipped  bool   `json:"skipped"`
			Stream   bool   `json:"stream"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return err
	}
	if body.SourceWillBeModified {
		return fmt.Errorf("%s must keep source immutable", phase)
	}
	if len(body.Blockers) > 0 {
		return fmt.Errorf("%s reported blockers: %v", phase, body.Blockers)
	}
	if len(body.Results) == 0 {
		return fmt.Errorf("%s returned no execution results", phase)
	}
	streamSeen := false
	for _, result := range body.Results {
		if result.Error != "" {
			return fmt.Errorf("%s failed %s: %s", phase, result.ActionID, result.Error)
		}
		if result.DryRun || result.Skipped {
			return fmt.Errorf("%s returned dry-run or skipped result for %s", phase, result.ActionID)
		}
		if result.Stream {
			streamSeen = true
		}
	}
	if expectStream && !streamSeen {
		return fmt.Errorf("%s did not execute any stream action", phase)
	}
	return nil
}

func (r runner) captureSourceSnapshot(ctx context.Context, sourceAlias, sshConfigPath string) (string, error) {
	result, err := r.runCommand(ctx, "ssh", []string{"-F", sshConfigPath, sourceAlias, "sha256sum", "/srv/hostshift-fixture/public/health", "/srv/hostshift-fixture/systemd-marker", "/etc/nginx/sites-available/hostshift-fixture.conf", "/etc/apache2/ports.conf", "/etc/apache2/sites-available/hostshift-fixture.conf", "/etc/systemd/system/hostshift-fixture-app.service"}, runOptions{Capture: true, TimeoutMs: sshTimeoutMs})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

func (r runner) runHostShift(ctx context.Context, args []string, env []string) (commandResult, error) {
	command := r.resolveHostShiftCommand()
	fullArgs := append([]string{}, command.PrefixArgs...)
	fullArgs = append(fullArgs, args...)
	return r.runCommand(ctx, command.Command, fullArgs, runOptions{CWD: r.repoRoot, Env: env, Capture: true, TimeoutMs: hostshiftTimeoutMs})
}

func (r runner) resolveHostShiftCommand() hostshiftCommand {
	if override := os.Getenv("HOSTSHIFT_VM_HOSTSHIFT_BIN"); override != "" {
		return hostshiftCommand{Command: override}
	}
	builtBinary := filepath.Join(r.repoRoot, "dist", "hostshift")
	if info, err := os.Stat(builtBinary); err == nil && !info.IsDir() {
		return hostshiftCommand{Command: builtBinary}
	}
	return hostshiftCommand{Command: "go", PrefixArgs: []string{"run", "./cmd/hostshift"}}
}

func (r runner) runProviderPreflight(ctx context.Context, providerName string, provider providerConfig) error {
	if os.Getenv("HOSTSHIFT_RUN_VM_E2E") != "1" {
		return nil
	}
	for _, bin := range provider.RequiredBinaries {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("missing required binary for provider %s: %s", providerName, bin)
		}
	}
	if providerName == "lima" {
		result, err := r.runCommand(ctx, "limactl", []string{"--version"}, runOptions{Capture: true, TimeoutMs: limactlTimeoutMs})
		if err != nil {
			return err
		}
		if version := strings.TrimSpace(result.Stdout); version != "" {
			fmt.Fprintf(r.stdout, "Lima preflight: %s\n", version)
		}
	}
	return nil
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
		return commandResult{}, fmt.Errorf("%s timed out after %dms", shellCommand(command, args), timeout)
	}
	if err != nil {
		detail := strings.TrimSpace(firstNonEmpty(stderr.String(), stdout.String(), err.Error()))
		return commandResult{}, fmt.Errorf("%s", detail)
	}
	return commandResult{Stdout: stdout.String(), Stderr: stderr.String()}, nil
}

func (r runner) logStep(workspace pairWorkspace, message string) {
	fmt.Fprintf(r.stdout, "[%s->%s] %s\n", workspace.Pair.Source, workspace.Pair.Target, message)
}

func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(cwd, "tests", "e2e", "vm", "matrix.yaml")); err == nil {
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

func copyFixture(sourcePath, destPath string) error {
	body, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	return os.WriteFile(destPath, body, 0o755)
}

func writeJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func replaceTemplate(template string, variables map[string]string) string {
	out := template
	for key, value := range variables {
		out = strings.ReplaceAll(out, "{{"+key+"}}", value)
	}
	return out
}

func sanitizeInstanceName(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('-')
		}
	}
	return builder.String()
}

func stripWrappedQuotes(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func yamlQuote(value string) string {
	body, _ := json.Marshal(value)
	return string(body)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
