//go:build !linux

package application

import "context"

// KillProcessesInWorkspace es un no-op fuera de Linux. El helper real
// vive en process_cleanup_linux.go y lee /proc/<pid>/{cwd,status}.
func KillProcessesInWorkspace(ctx context.Context, workspacePath string) (int, error) {
	return 0, nil
}