package cmd

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

func runSSHScript(target, script string) (string, error) {
	return runSSHScriptWithTimeout(target, script, 30*time.Second)
}

func runSSHScriptWithTimeout(target, script string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ssh",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		target,
		"bash", "-s",
	)
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return strings.TrimSpace(string(out)), context.DeadlineExceeded
	}
	return strings.TrimSpace(string(out)), err
}
