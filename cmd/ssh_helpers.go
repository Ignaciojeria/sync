package cmd

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

const sshHostKeysNoisePrefix = "client_global_hostkeys_prove_confirm: server gave bad signature for "

func newSSHCommand(args ...string) *exec.Cmd {
	allArgs := append([]string{"-o", "UpdateHostKeys=no"}, args...)
	return exec.Command("ssh", allArgs...)
}

func newSSHCommandContext(ctx context.Context, args ...string) *exec.Cmd {
	allArgs := append([]string{"-o", "UpdateHostKeys=no"}, args...)
	return exec.CommandContext(ctx, "ssh", allArgs...)
}

func newSCPCommand(args ...string) *exec.Cmd {
	allArgs := append([]string{"-o", "UpdateHostKeys=no"}, args...)
	return exec.Command("scp", allArgs...)
}

func stripSSHNoise(output string) string {
	if strings.TrimSpace(output) == "" {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(output, "\r", ""), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, sshHostKeysNoisePrefix) {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

func runSSHScript(target, script string) (string, error) {
	return runSSHScriptWithTimeout(target, script, 30*time.Second)
}

func runSSHScriptWithTimeout(target, script string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := newSSHCommandContext(ctx,
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		target,
		"bash", "-s",
	)
	cmd.Stdin = strings.NewReader(script)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stripSSHNoise(stdout.String())
	errOut := stripSSHNoise(stderr.String())
	combined := out
	if errOut != "" {
		if combined != "" {
			combined += "\n"
		}
		combined += errOut
	}
	if ctx.Err() == context.DeadlineExceeded {
		return strings.TrimSpace(combined), context.DeadlineExceeded
	}
	if err != nil {
		return strings.TrimSpace(combined), err
	}
	return strings.TrimSpace(out), nil
}
