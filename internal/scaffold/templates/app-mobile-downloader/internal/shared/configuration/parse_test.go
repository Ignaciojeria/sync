package configuration

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestFindProjectRoot(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() { _ = os.Chdir(wd) }()

	dir := t.TempDir()
	root := filepath.Join(dir, "repo")
	nested := filepath.Join(root, "deep", "child")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n\ngo 1.25.0\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	got, err := filepath.EvalSymlinks(findProjectRoot())
	if err != nil {
		t.Fatalf("EvalSymlinks(got) error = %v", err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(root) error = %v", err)
	}
	if got != want {
		t.Fatalf("findProjectRoot() = %q, want %q", got, want)
	}
}

func TestParse(t *testing.T) {
	t.Run("parses environment variables", func(t *testing.T) {
		once = sync.Once{}
		t.Setenv("TEST_NAME", "mobile-downloader")
		t.Setenv("TEST_PORT", "8080")

		type conf struct {
			Name string `env:"TEST_NAME"`
			Port int    `env:"TEST_PORT"`
		}

		got, err := Parse[conf]()
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if got.Name != "mobile-downloader" || got.Port != 8080 {
			t.Fatalf("unexpected conf: %+v", got)
		}
	})

	t.Run("returns parse errors", func(t *testing.T) {
		once = sync.Once{}
		t.Setenv("TEST_BAD_INT", "not-a-number")

		type conf struct {
			Bad int `env:"TEST_BAD_INT"`
		}

		if _, err := Parse[conf](); err == nil {
			t.Fatal("expected parse error")
		}
	})
}
