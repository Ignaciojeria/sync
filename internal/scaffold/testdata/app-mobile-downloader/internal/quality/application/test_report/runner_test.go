package testreport

import (
	"errors"
	"testing"

	"app-mobile-downloader/internal/quality/ui"
)

func baseDeps() RunnerDeps {
	return RunnerDeps{
		FindProjectRoot:            func() (string, error) { return "/root", nil },
		EnsureCoverageDir:          func() error { return nil },
		RunTests:                   func(root, coverProfile string) ([]byte, error) { return []byte("ok"), nil },
		FilterCoverageFile:         func(input, output string) error { return nil },
		CoveragePercentFromProfile: func(root, profile string) (float64, error) { return 76.5, nil },
		GenerateHTMLReport:         func(root, filteredProfile, htmlReport string) error { return nil },
		SaveLastRunState:           func(state ui.TestRunState) error { return nil },
		IsAllowedEditorEmail:       func(email string) bool { return true },
	}
}

func TestRunnerUnauthorized(t *testing.T) {
	deps := baseDeps()
	deps.IsAllowedEditorEmail = func(email string) bool { return false }
	_, err := NewRunnerWithDeps(deps).Run("bad@example.com")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "forbidden" {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestRunnerFindProjectRootError(t *testing.T) {
	deps := baseDeps()
	deps.FindProjectRoot = func() (string, error) { return "", errors.New("root error") }
	_, err := NewRunnerWithDeps(deps).Run("ok@example.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunnerEnsureCoverageDirError(t *testing.T) {
	deps := baseDeps()
	deps.EnsureCoverageDir = func() error { return errors.New("dir error") }
	_, err := NewRunnerWithDeps(deps).Run("ok@example.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunnerGoTestFailure(t *testing.T) {
	deps := baseDeps()
	deps.RunTests = func(root, coverProfile string) ([]byte, error) {
		return []byte("FAIL"), errors.New("test failed")
	}
	calledSave := false
	deps.SaveLastRunState = func(state ui.TestRunState) error {
		calledSave = true
		return nil
	}
	state, err := NewRunnerWithDeps(deps).Run("ok@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !calledSave {
		t.Fatal("expected SaveLastRunState to be called")
	}
	if state.Success {
		t.Fatal("expected failure state")
	}
}

func TestRunnerFilterCoverageFileError(t *testing.T) {
	deps := baseDeps()
	deps.RunTests = func(root, coverProfile string) ([]byte, error) { return []byte("ok"), nil }
	deps.FilterCoverageFile = func(input, output string) error { return errors.New("filter error") }
	_, err := NewRunnerWithDeps(deps).Run("ok@example.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunnerCoveragePercentFromProfileError(t *testing.T) {
	deps := baseDeps()
	deps.RunTests = func(root, coverProfile string) ([]byte, error) { return []byte("ok"), nil }
	deps.CoveragePercentFromProfile = func(root, profile string) (float64, error) {
		return 0, errors.New("cover error")
	}
	_, err := NewRunnerWithDeps(deps).Run("ok@example.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunnerGenerateHTMLReportError(t *testing.T) {
	deps := baseDeps()
	deps.RunTests = func(root, coverProfile string) ([]byte, error) { return []byte("ok"), nil }
	deps.GenerateHTMLReport = func(root, filteredProfile, htmlReport string) error { return errors.New("html error") }
	_, err := NewRunnerWithDeps(deps).Run("ok@example.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunnerSuccess(t *testing.T) {
	deps := baseDeps()
	deps.RunTests = func(root, coverProfile string) ([]byte, error) { return []byte("ok"), nil }
	calledSave := false
	deps.SaveLastRunState = func(state ui.TestRunState) error {
		if !state.Success {
			t.Fatal("expected success state")
		}
		calledSave = true
		return nil
	}
	state, err := NewRunnerWithDeps(deps).Run("ok@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !calledSave {
		t.Fatal("expected SaveLastRunState to be called")
	}
	if !state.Success {
		t.Fatal("expected success state")
	}
	if state.CoverPath == "" {
		t.Fatal("expected cover path")
	}
}

func TestRunnerGoTestFailureStillReturnsState(t *testing.T) {
	deps := baseDeps()
	deps.RunTests = func(root, coverProfile string) ([]byte, error) {
		return []byte("FAIL"), errors.New("test failed")
	}
	state, err := NewRunnerWithDeps(deps).Run("ok@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Success {
		t.Fatal("expected failure state")
	}
	if state.CoverPercent != 0 {
		t.Fatal("expected zero coverage")
	}
}
