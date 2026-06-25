package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSSHArgsWithProjectIdentity_UsesProjectKeyForVMTarget(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	keyPath := filepath.Join(tmp, ".einar", "id_ed25519")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("dummy-private-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	args := sshArgsWithProjectIdentity("-o", "BatchMode=yes", "exedev@aaaachhh2.exe.xyz", "echo", "ok")
	if len(args) < 6 {
		t.Fatalf("unexpected args: %v", args)
	}
	wantPrefix := []string{"-i", keyPath, "-o", "IdentitiesOnly=yes"}
	if !reflect.DeepEqual(args[:4], wantPrefix) {
		t.Fatalf("expected prefix %v, got %v", wantPrefix, args[:4])
	}
}

func TestSSHArgsWithProjectIdentity_DoesNotUseProjectKeyForExeDev(t *testing.T) {
	args := sshArgsWithProjectIdentity("exe.dev", "exit")
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-i" {
			t.Fatalf("did not expect project identity for exe.dev, got args %v", args)
		}
	}
}

func TestDetectSSHInvocationTarget(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "skips option values",
			args: []string{"-o", "BatchMode=yes", "-p", "22", "user@host", "echo", "ok"},
			want: "user@host",
		},
		{
			name: "returns empty without target",
			args: []string{"-o", "BatchMode=yes"},
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectSSHInvocationTarget(tc.args); got != tc.want {
				t.Fatalf("detectSSHInvocationTarget(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestDetectSCPInvocationTarget(t *testing.T) {
	got := detectSCPInvocationTarget([]string{"-r", "local-dir", "exedev@aaaachhh2.exe.xyz:/remote/path"})
	if got != "exedev@aaaachhh2.exe.xyz" {
		t.Fatalf("unexpected scp target: %q", got)
	}
}
