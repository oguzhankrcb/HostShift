package ssh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/oguzhankaracabay/hostshift/internal/safety"
)

type Runner struct{}

func (Runner) Run(ctx context.Context, alias string, remoteCommand []string) ([]byte, error) {
	return (Runner{}).RunSource(ctx, alias, remoteCommand)
}

func (Runner) RunSource(ctx context.Context, alias string, remoteCommand []string) ([]byte, error) {
	if err := safety.SSHAlias(alias); err != nil {
		return nil, err
	}
	if err := safety.SourceCommand(remoteCommand); err != nil {
		return nil, err
	}
	args := append(sshBaseArgs(alias), joinRemoteCommand(remoteCommand))
	cmd := exec.CommandContext(ctx, "ssh", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ssh source command failed: %w: %s", err, strings.TrimSpace(coalesce(stderr.String(), stdout.String())))
	}
	return stdout.Bytes(), nil
}

func (Runner) RunTarget(ctx context.Context, alias string, remoteCommand []string) ([]byte, error) {
	if err := safety.SSHAlias(alias); err != nil {
		return nil, err
	}
	if err := safety.TargetCommand(remoteCommand); err != nil {
		return nil, err
	}
	remoteCommand = targetCommand(remoteCommand)
	args := append(sshBaseArgs(alias), joinRemoteCommand(remoteCommand))
	cmd := exec.CommandContext(ctx, "ssh", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ssh target command failed: %w: %s", err, strings.TrimSpace(coalesce(stderr.String(), stdout.String())))
	}
	return stdout.Bytes(), nil
}

func (Runner) Stream(ctx context.Context, sourceAlias string, sourceCommand []string, targetAlias string, targetCommand []string) ([]byte, error) {
	if err := safety.SSHAlias(sourceAlias); err != nil {
		return nil, err
	}
	if err := safety.SSHAlias(targetAlias); err != nil {
		return nil, err
	}
	if err := safety.SourceCommand(sourceCommand); err != nil {
		return nil, err
	}
	if err := safety.TargetCommand(targetCommand); err != nil {
		return nil, err
	}
	sourceArgs := append(sshBaseArgs(sourceAlias), joinRemoteCommand(sourceCommand))
	targetCommand = targetCommandForStream(targetCommand)
	targetArgs := append(sshBaseArgs(targetAlias), joinRemoteCommand(targetCommand))
	source := exec.CommandContext(ctx, "ssh", sourceArgs...)
	target := exec.CommandContext(ctx, "ssh", targetArgs...)
	pipe, err := source.StdoutPipe()
	if err != nil {
		return nil, err
	}
	target.Stdin = pipe
	var stderr bytes.Buffer
	source.Stderr = &stderr
	target.Stderr = &stderr
	var stdout bytes.Buffer
	target.Stdout = &stdout
	if err := target.Start(); err != nil {
		return nil, err
	}
	if err := source.Start(); err != nil {
		_ = target.Process.Kill()
		return nil, err
	}
	sourceErr := source.Wait()
	if closer, ok := pipe.(io.Closer); ok {
		_ = closer.Close()
	}
	targetErr := target.Wait()
	if sourceErr != nil {
		return nil, fmt.Errorf("source stream failed: %w: %s", sourceErr, stderr.String())
	}
	if targetErr != nil {
		return nil, fmt.Errorf("target stream failed: %w: %s", targetErr, stderr.String())
	}
	return stdout.Bytes(), nil
}

func sshBaseArgs(alias string) []string {
	args := []string{}
	if configPath := os.Getenv("HOSTSHIFT_SSH_CONFIG"); configPath != "" {
		args = append(args, "-F", configPath)
	}
	args = append(args, alias)
	return args
}

func targetCommand(command []string) []string {
	if os.Getenv("HOSTSHIFT_TARGET_SUDO") != "1" {
		return command
	}
	return append([]string{"sudo", "--non-interactive", "--"}, command...)
}

func targetCommandForStream(command []string) []string {
	if os.Getenv("HOSTSHIFT_TARGET_SUDO") != "1" {
		return command
	}
	return append([]string{"sudo", "--non-interactive", "--"}, command...)
}

func joinRemoteCommand(command []string) string {
	quoted := make([]string, len(command))
	for index, arg := range command {
		quoted[index] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func coalesce(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
