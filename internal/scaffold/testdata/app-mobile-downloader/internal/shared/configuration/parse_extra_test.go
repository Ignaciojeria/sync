package configuration

import (
	"os"
	"sync"
	"testing"
)

func TestFindProjectRootFallbackToCwd(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd(): %v", err)
	}
	defer func() { _ = os.Chdir(wd) }()

	// t.TempDir() does not contain a go.mod; findProjectRoot should fall back to the working directory.
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir(): %v", err)
	}
	if got := findProjectRoot(); got == "" {
		t.Fatal("expected fallback to cwd")
	}
}

func TestLoadEnvOnceWithMissingEnvFile(t *testing.T) {
	once = sync.Once{}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd(): %v", err)
	}
	defer func() { _ = os.Chdir(wd) }()

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir(): %v", err)
	}

	loadEnvOnce()
	loadEnvOnce() // second call exercises the cached once state.
}
