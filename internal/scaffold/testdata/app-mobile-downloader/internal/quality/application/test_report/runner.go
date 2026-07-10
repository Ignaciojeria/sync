package testreport

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"scaffoldxd1/internal/shared"
	infratest "scaffoldxd1/internal/shared/infrastructure/test"
	"scaffoldxd1/internal/quality/ui"
)

type RunnerDeps struct {
	FindProjectRoot          func() (string, error)
	EnsureCoverageDir        func() error
	RunTests                 func(root, coverProfile string) ([]byte, error)
	FilterCoverageFile       func(input, output string) error
	CoveragePercentFromProfile func(root, profile string) (float64, error)
	GenerateHTMLReport       func(root, filteredProfile, htmlReport string) error
	SaveLastRunState         func(state ui.TestRunState) error
	IsAllowedEditorEmail     func(string) bool
}

type Runner struct {
	deps RunnerDeps
}

func NewRunner() *Runner {
	return &Runner{
		deps: RunnerDeps{
			FindProjectRoot:          infratest.FindProjectRoot,
			EnsureCoverageDir:        func() error { return infratest.EnsureCoverageDir(infratest.CoverageDir) },
			RunTests:                 infratest.RunTests,
			FilterCoverageFile:       infratest.FilterCoverageFile,
			CoveragePercentFromProfile: func(root, profile string) (float64, error) {
				return infratest.CoveragePercentFromProfile(root, profile, exec.Command)
			},
			GenerateHTMLReport:       infratest.GenerateHTMLReport,
			SaveLastRunState:         infratest.SaveLastRunState,
			IsAllowedEditorEmail:     shared.IsAllowedEditorEmail,
		},
	}
}

func NewRunnerWithDeps(deps RunnerDeps) *Runner {
	return &Runner{deps: deps}
}

func (r *Runner) Run(email string) (ui.TestRunState, error) {
	if !r.deps.IsAllowedEditorEmail(email) {
		return ui.TestRunState{}, fmt.Errorf("forbidden")
	}

	root, err := r.deps.FindProjectRoot()
	if err != nil {
		return ui.TestRunState{}, fmt.Errorf("find project root: %w", err)
	}

	if err := r.deps.EnsureCoverageDir(); err != nil {
		return ui.TestRunState{}, fmt.Errorf("ensure coverage dir: %w", err)
	}

	coverProfile := filepath.Join(infratest.CoverageDir, "coverage.out")
	htmlReport := filepath.Join(infratest.CoverageDir, "coverage.html")

	_ = os.Remove(coverProfile)
	_ = os.Remove(htmlReport)

	testOutput, testErr := r.deps.RunTests(root, coverProfile)

	output := string(testOutput)
	var coverPercent float64
	var coverPath string
	var success bool

	if testErr != nil {
		state := ui.TestRunState{
			Success:      false,
			Output:       output,
			CoverPercent: 0,
			Timestamp:    time.Now(),
			HasResult:    true,
		}
		_ = r.deps.SaveLastRunState(state)
		return state, nil
	}

	filteredProfile := filepath.Join(infratest.CoverageDir, "coverage_filtered.out")
	if err := r.deps.FilterCoverageFile(coverProfile, filteredProfile); err != nil {
		return ui.TestRunState{}, fmt.Errorf("filter coverage: %w", err)
	}

	coverPercent, err = r.deps.CoveragePercentFromProfile(root, filteredProfile)
	if err != nil {
		return ui.TestRunState{}, fmt.Errorf("coverage percent: %w", err)
	}

	if err := r.deps.GenerateHTMLReport(root, filteredProfile, htmlReport); err != nil {
		return ui.TestRunState{}, fmt.Errorf("generate html report: %w", err)
	}

	coverPath = fmt.Sprintf("/report/tests/coverage.html?t=%d", time.Now().UnixNano())
	success = true

	state := ui.TestRunState{
		Success:      success,
		Output:       output,
		CoverPath:    coverPath,
		CoverPercent: coverPercent,
		Timestamp:    time.Now(),
		HasResult:    true,
	}
	_ = r.deps.SaveLastRunState(state)
	return state, nil
}



