package test

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"app-mobile-downloader/internal/quality/ui"
)

func TestFilterCoverageFile(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "coverage.out")
	output := filepath.Join(dir, "coverage_filtered.out")
	content := strings.Join([]string{
		"mode: set",
		"templates/layout_templ.go:1.1,2.2 1 1",
		"cmd/api/main.go:1.1,2.2 1 1",
		"internal/shared/infrastructure/postgresql/connection.go:1.1,2.2 1 1",
		"internal/shared/access.go:1.1,2.2 1 1",
	}, "\n")
	if err := os.WriteFile(input, []byte(content), 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	if err := FilterCoverageFile(input, output); err != nil {
		t.Fatalf("FilterCoverageFile() error = %v", err)
	}

	if err := FilterCoverageFile(filepath.Join(dir, "missing.out"), output); err == nil {
		t.Fatal("expected read error for missing input file")
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	got := string(data)
	if strings.Contains(got, "_templ.go") {
		t.Fatalf("expected generated templ files to be removed: %q", got)
	}
	if strings.Contains(got, "cmd/api/main.go") {
		t.Fatalf("expected cmd/api/main.go to be removed: %q", got)
	}
	if strings.Contains(got, "internal/shared/infrastructure/postgresql/connection.go") {
		t.Fatalf("expected connection.go to be removed: %q", got)
	}
	if !strings.Contains(got, "internal/shared/access.go") {
		t.Fatalf("expected non-templ lines to remain: %q", got)
	}
}

func TestShouldExcludeCoverageLine(t *testing.T) {
	if !ShouldExcludeCoverageLine([]byte("templates/layout_templ.go:1.1,2.2 1 1")) {
		t.Fatal("expected templ generated file to be excluded")
	}
	if !ShouldExcludeCoverageLine([]byte("cmd/api/main.go:1.1,2.2 1 1")) {
		t.Fatal("expected main.go to be excluded")
	}
	if !ShouldExcludeCoverageLine([]byte("internal/shared/infrastructure/postgresql/connection.go:1.1,2.2 1 1")) {
		t.Fatal("expected connection.go to be excluded")
	}
	if ShouldExcludeCoverageLine([]byte("internal/shared/access.go:1.1,2.2 1 1")) {
		t.Fatal("did not expect regular source file to be excluded")
	}
}

func TestFindProjectRoot(t *testing.T) {
	got, err := FindProjectRoot()
	if err != nil {
		t.Fatalf("FindProjectRoot() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(got, "go.mod")); err != nil {
		t.Fatalf("expected go.mod in root, got %q", got)
	}
}

func TestFindProjectRootFromNested(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "repo")
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n\ngo 1.25.0\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	got, err := FindProjectRootFrom(nested, os.Getwd, os.Stat)
	if err != nil {
		t.Fatalf("FindProjectRootFrom() error = %v", err)
	}
	if got != root {
		t.Fatalf("root = %q, want %q", got, root)
	}
}

func TestFindProjectRootFromGetwdError(t *testing.T) {
	if _, err := FindProjectRootFrom("", func() (string, error) { return "", errors.New("getwd failed") }, os.Stat); err == nil {
		t.Fatal("expected error for getwd failure")
	}
}

func TestFindProjectRootNotFound(t *testing.T) {
	if _, err := FindProjectRootFrom(t.TempDir(), os.Getwd, func(name string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}); err == nil {
		t.Fatal("expected error when go.mod not found")
	}
}

func TestParseCoverPercent(t *testing.T) {
	if got := ParseCoverPercent("ok pkg coverage: 87.5% of statements\n"); got != 87.5 {
		t.Fatalf("ParseCoverPercent() = %v", got)
	}
	if got := ParseCoverPercent("total: (statements) 91.3%\n"); got != 91.3 {
		t.Fatalf("ParseCoverPercent() from go tool cover = %v", got)
	}
	if got := ParseCoverPercent("no coverage output"); got != 0 {
		t.Fatalf("expected zero when coverage is absent, got %v", got)
	}
}

func TestCoveragePercentFromProfile(t *testing.T) {
	t.Run("command error", func(t *testing.T) {
		if _, err := CoveragePercentFromProfile(t.TempDir(), filepath.Join(t.TempDir(), "missing.out"), exec.Command); err == nil {
			t.Fatal("expected error for invalid coverage profile")
		}
	})

	t.Run("successful parse", func(t *testing.T) {
		t.Setenv("GO_HELPER_PROCESS", "1")
		dir := t.TempDir()
		got, err := CoveragePercentFromProfile(dir, filepath.Join(dir, "coverage.out"), func(name string, args ...string) *exec.Cmd {
			return exec.Command(os.Args[0], "-test.run=TestHelperFakeGoToolCover")
		})
		if err != nil {
			t.Fatalf("CoveragePercentFromProfile() error = %v", err)
		}
		if got != 76.5 {
			t.Fatalf("CoveragePercentFromProfile() = %v, want 76.5", got)
		}
	})
}

func TestHelperFakeGoToolCover(t *testing.T) {
	if os.Getenv("GO_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Println("total: (statements) 76.5%")
	os.Exit(0)
}

func TestSaveAndLoadLastRunState(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() { _ = os.Chdir(wd) }()

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	if err := os.MkdirAll(CoverageDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	want := ui.TestRunState{
		Success:      true,
		Output:       "all good",
		CoverPath:    "/report/tests/coverage.html?t=123",
		CoverPercent: 99.9,
		Timestamp:    time.Unix(1700000000, 0).UTC(),
		HasResult:    true,
	}
	if err := SaveLastRunState(want); err != nil {
		t.Fatalf("SaveLastRunState() error = %v", err)
	}

	got := LoadLastRunState()
	if got.Success != want.Success || got.Output != want.Output || got.CoverPath != want.CoverPath || got.CoverPercent != want.CoverPercent || !got.Timestamp.Equal(want.Timestamp) || got.HasResult != want.HasResult {
		t.Fatalf("loaded state = %+v, want %+v", got, want)
	}

	if err := SaveLastRunState(ui.TestRunState{CoverPercent: math.NaN()}); err == nil {
		t.Fatal("expected marshal error for NaN coverage percent")
	}
}

func TestLoadLastRunStateInvalidJSON(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() { _ = os.Chdir(wd) }()

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	if err := os.MkdirAll(CoverageDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(CoverageDir, "last_run.json"), []byte("not-json"), 0644); err != nil {
		t.Fatalf("write invalid json: %v", err)
	}

	got := LoadLastRunState()
	if got != (ui.TestRunState{}) {
		t.Fatalf("expected zero value state, got %+v", got)
	}

	_ = os.Remove(filepath.Join(CoverageDir, "last_run.json"))
	got = LoadLastRunState()
	if got != (ui.TestRunState{}) {
		t.Fatalf("expected zero value state when file is missing, got %+v", got)
	}
}

func TestEnsureCoverageDir(t *testing.T) {
	t.Run("creates directory", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "coverage")
		if err := EnsureCoverageDir(target); err != nil {
			t.Fatalf("EnsureCoverageDir() error = %v", err)
		}
		info, err := os.Stat(target)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if !info.IsDir() {
			t.Fatal("expected directory")
		}
	})

	t.Run("removes file and creates directory", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "coverage")
		if err := os.WriteFile(target, []byte("file"), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := EnsureCoverageDir(target); err != nil {
			t.Fatalf("EnsureCoverageDir() error = %v", err)
		}
		info, err := os.Stat(target)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if !info.IsDir() {
			t.Fatal("expected directory after removing file")
		}
	})

	t.Run("mkdirall error", func(t *testing.T) {
		dir := t.TempDir()
		parent := filepath.Join(dir, "parent")
		if err := os.WriteFile(parent, []byte("file"), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		target := filepath.Join(parent, "coverage")
		if err := EnsureCoverageDir(target); err == nil {
			t.Fatal("expected error when parent is a file")
		}
	})
}

func TestRunTests(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/tmp\n\ngo 1.25.0\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "calc.go"), []byte("package tmp\n\nfunc Add(a, b int) int { return a + b }\n"), 0644); err != nil {
		t.Fatalf("write calc.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "calc_test.go"), []byte("package tmp\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"unexpected result\")\n\t}\n}\n"), 0644); err != nil {
		t.Fatalf("write calc_test.go: %v", err)
	}

	coverProfile := filepath.Join(dir, "coverage.out")
	out, err := RunTests(dir, coverProfile)
	if err != nil {
		t.Fatalf("RunTests() error = %v, output=%s", err, string(out))
	}
	if len(out) == 0 {
		t.Fatal("expected go test output")
	}
	if _, err := os.Stat(coverProfile); err != nil {
		t.Fatalf("expected coverage profile to exist: %v", err)
	}

	if _, err := RunTests(filepath.Join(dir, "missing"), coverProfile); err == nil {
		t.Fatal("expected error for missing root")
	}
}

func TestGenerateHTMLReport(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/tmp\n\ngo 1.25.0\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "calc.go"), []byte("package tmp\n\nfunc Add(a, b int) int { return a + b }\n"), 0644); err != nil {
		t.Fatalf("write calc.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "calc_test.go"), []byte("package tmp\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"unexpected result\")\n\t}\n}\n"), 0644); err != nil {
		t.Fatalf("write calc_test.go: %v", err)
	}

	coverProfile := filepath.Join(dir, "coverage.out")
	if out, err := RunTests(dir, coverProfile); err != nil {
		t.Fatalf("RunTests() error = %v, output=%s", err, string(out))
	}

	htmlReport := filepath.Join(dir, "coverage.html")
	if err := GenerateHTMLReport(dir, coverProfile, htmlReport); err != nil {
		t.Fatalf("GenerateHTMLReport() error = %v", err)
	}
	if _, err := os.Stat(htmlReport); err != nil {
		t.Fatalf("expected html report to exist: %v", err)
	}

	if err := GenerateHTMLReport(dir, filepath.Join(dir, "missing.out"), filepath.Join(dir, "missing.html")); err == nil {
		t.Fatal("expected error for missing profile")
	}
}
