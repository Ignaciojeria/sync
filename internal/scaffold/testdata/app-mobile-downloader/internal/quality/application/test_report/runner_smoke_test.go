package testreport

import (
	"os"
	"path/filepath"
	"testing"

	"app-mobile-downloader/internal/quality/ui"
)

// TestNewRunner wires the production Runner with the real dependencies from
// the support package and confirms every dependency is reachable so coverage
// tracks each assignment and closure body. The test operates from a sandboxed
// temp directory so the wired dependencies can write their results without
// polluting the project tree.
//
//nolint:paralleltest // covers constructor wiring only
func TestNewRunner(t *testing.T) {
	// Run in a temp dir so the wired deps (EnsureCoverageDir, SaveLastRunState,
	// etc.) write into the sandbox rather than the project's tmp/ directory.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	tmp, err := os.MkdirTemp("", "runner-smoke-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	r := NewRunner()
	if r == nil {
		t.Fatal("expected runner to be created by NewRunner")
	}
	if r.deps.FindProjectRoot == nil {
		t.Fatal("FindProjectRoot dependency was not wired")
	}
	if r.deps.EnsureCoverageDir == nil {
		t.Fatal("EnsureCoverageDir dependency was not wired")
	}
	if r.deps.RunTests == nil {
		t.Fatal("RunTests dependency was not wired")
	}
	if r.deps.FilterCoverageFile == nil {
		t.Fatal("FilterCoverageFile dependency was not wired")
	}
	if r.deps.CoveragePercentFromProfile == nil {
		t.Fatal("CoveragePercentFromProfile dependency was not wired")
	}
	if r.deps.GenerateHTMLReport == nil {
		t.Fatal("GenerateHTMLReport dependency was not wired")
	}
	if r.deps.SaveLastRunState == nil {
		t.Fatal("SaveLastRunState dependency was not wired")
	}
	if r.deps.IsAllowedEditorEmail == nil {
		t.Fatal("IsAllowedEditorEmail dependency was not wired")
	}

	// Exercise each wired dependency so coverage tracks their closure bodies
	// too. Inputs are deliberately minimal or non-existent so the wrappers
	// return errors quickly without blocking.
	_, _ = r.deps.FindProjectRoot()
	_ = r.deps.EnsureCoverageDir() // creates tmp/coverage inside our temp dir

	if _, err := os.Stat(filepath.Join(tmp, "tmp", "coverage")); err != nil {
		t.Fatalf("expected EnsureCoverageDir to create dir: %v", err)
	}

	_ = r.deps.FilterCoverageFile(filepath.Join(tmp, "missing.in"), filepath.Join(tmp, "missing.out"))
	_, _ = r.deps.CoveragePercentFromProfile(tmp, filepath.Join(tmp, "missing.out"))
	_ = r.deps.GenerateHTMLReport(tmp, filepath.Join(tmp, "missing.out"), filepath.Join(tmp, "missing.html"))
	_ = r.deps.SaveLastRunState(ui.TestRunState{})
	_ = r.deps.IsAllowedEditorEmail("dev@example.com")
	_ = r.deps.IsAllowedEditorEmail("nobody@example.com")
}
