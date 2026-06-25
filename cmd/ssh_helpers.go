package cmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

const sshHostKeysNoisePrefix = "client_global_hostkeys_prove_confirm: server gave bad signature for "

func newSSHCommand(args ...string) *exec.Cmd {
	return exec.Command("ssh", sshArgsWithProjectIdentity(args...)...)
}

func newSSHCommandContext(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "ssh", sshArgsWithProjectIdentity(args...)...)
}

func newSCPCommand(args ...string) *exec.Cmd {
	return exec.Command("scp", scpArgsWithProjectIdentity(args...)...)
}

func sshArgsWithProjectIdentity(args ...string) []string {
	allArgs := append([]string{"-o", "UpdateHostKeys=no"}, args...)
	target := detectSSHInvocationTarget(args)
	if keyPath, ok := projectSSHIdentityFileForTarget(target); ok {
		prefix := []string{"-i", keyPath, "-o", "IdentitiesOnly=yes"}
		allArgs = append(prefix, allArgs...)
	}
	return allArgs
}

func scpArgsWithProjectIdentity(args ...string) []string {
	allArgs := append([]string{"-o", "UpdateHostKeys=no"}, args...)
	target := detectSCPInvocationTarget(args)
	if keyPath, ok := projectSSHIdentityFileForTarget(target); ok {
		prefix := []string{"-i", keyPath, "-o", "IdentitiesOnly=yes"}
		allArgs = append(prefix, allArgs...)
	}
	return allArgs
}

func projectSSHIdentityFileForTarget(target string) (string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false
	}
	host := target
	if i := strings.Index(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	host = strings.TrimSpace(host)
	if host == "" || strings.EqualFold(host, "exe.dev") {
		return "", false
	}
	keyPath, err := projectSSHPrivateKeyPath()
	if err != nil || strings.TrimSpace(keyPath) == "" {
		return "", false
	}
	if st, err := os.Stat(keyPath); err != nil || st.IsDir() || st.Size() == 0 {
		return "", false
	}
	return keyPath, true
}

func detectSSHInvocationTarget(args []string) string {
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "-o" || arg == "-i" || arg == "-F" || arg == "-J" || arg == "-l" || arg == "-p" || arg == "-W" || arg == "-b" || arg == "-c" || arg == "-D" || arg == "-E" || arg == "-e" || arg == "-I" || arg == "-L" || arg == "-m" || arg == "-O" || arg == "-Q" || arg == "-R" || arg == "-S" || arg == "-w" {
			skipNext = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg
	}
	return ""
}

func detectSCPInvocationTarget(args []string) string {
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" || strings.HasPrefix(trimmed, "-") {
			continue
		}
		if strings.Contains(trimmed, "@") && strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 2)
			return parts[0]
		}
	}
	return ""
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
