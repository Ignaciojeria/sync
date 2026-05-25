package cmd

import (
	"os/exec"
	"strings"
)

func runSSHScript(target, script string) (string, error) {
	cmd := exec.Command("ssh", target, "bash", "-s")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
