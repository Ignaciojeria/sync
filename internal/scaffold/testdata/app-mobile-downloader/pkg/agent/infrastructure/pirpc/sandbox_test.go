package pirpc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeSessionID(t *testing.T) {
	cases := map[string]string{
		"agent-1783013382907611187": "agent-1783013382907611187",
		"plain id":                  "plain-id",
		"weird/with\\slashes":       "weird-with-slashes",
		"":                          "",
		"---":                       "",
		"a.b_c-d":                   "a.b_c-d",
		"café":                      "caf-",
	}
	for in, want := range cases {
		if got := sanitizeSessionID(in); got != want {
			t.Errorf("sanitizeSessionID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveCWD_DefaultSandbox(t *testing.T) {
	// Cambiamos el cwd del test a un tempdir para que la creación del
	// sandbox no ensucie el repo.
	prev, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	for _, cwd := range []string{"", ".", "./", "  "} {
		got, err := resolveCWD(cwd, "agent-test-001")
		if err != nil {
			t.Fatalf("resolveCWD(%q): %v", cwd, err)
		}
		if !strings.HasSuffix(got, filepath.Join("tmp", "agent-work", "agent-test-001")) {
			t.Errorf("resolveCWD(%q) = %q, esperaba terminar en tmp/agent-work/agent-test-001", cwd, got)
		}
		info, err := os.Stat(got)
		if err != nil {
			t.Fatalf("sandbox no creado: %v", err)
		}
		if !info.IsDir() {
			t.Fatal("sandbox no es un directorio")
		}
	}
}

func TestResolveCWD_RespectsExplicitPath(t *testing.T) {
	prev, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(prev) })

	explicit, err := filepath.Abs("/tmp/explicit-cwd-for-pi")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	got, err := resolveCWD(explicit, "agent-test-ignored")
	if err != nil {
		t.Fatalf("resolveCWD: %v", err)
	}
	if got != explicit {
		t.Fatalf("got %q, want %q", got, explicit)
	}
}

func TestResolveCWD_RejectsEmptySessionIDWithDefault(t *testing.T) {
	prev, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	got, err := resolveCWD("", "")
	if err == nil {
		t.Fatalf("esperaba error para sessionID vacío, obtuve %q", got)
	}
}

func TestResolveCWD_AcceptsRelativeExplicitCWD(t *testing.T) {
	prev, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	got, err := resolveCWD("./explicit-relative", "agent-test-002")
	if err != nil {
		t.Fatalf("resolveCWD: %v", err)
	}
	if got != "./explicit-relative" {
		t.Fatalf("got %q, want %q", got, "./explicit-relative")
	}
}
