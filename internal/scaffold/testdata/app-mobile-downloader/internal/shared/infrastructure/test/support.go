package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"

	"app-mobile-downloader/internal/quality/ui"
)

const CoverageDir = "tmp/coverage"



func FilterCoverageFile(inputPath, outputPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	lines := bytes.Split(data, []byte("\n"))
	var filtered [][]byte
	for _, line := range lines {
		if ShouldExcludeCoverageLine(line) {
			continue
		}
		filtered = append(filtered, line)
	}
	return os.WriteFile(outputPath, bytes.Join(filtered, []byte("\n")), 0644)
}

func ShouldExcludeCoverageLine(line []byte) bool {
	return bytes.Contains(line, []byte("_templ.go")) ||
		bytes.Contains(line, []byte("cmd/api/main.go")) ||
		bytes.Contains(line, []byte("internal/shared/infrastructure/postgresql/connection.go"))
}

func FindProjectRoot() (string, error) {
	return FindProjectRootFrom("", os.Getwd, os.Stat)
}

func FindProjectRootFrom(dir string, getwd func() (string, error), stat func(string) (os.FileInfo, error)) (string, error) {
	if dir == "" {
		var err error
		dir, err = getwd()
		if err != nil {
			return "", err
		}
	}
	for {
		if _, err := stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("go.mod not found")
}

func ParseCoverPercent(output string) float64 {
	re := regexp.MustCompile(`(?:total:\s*\(statements\)\s*|coverage:\s*)([0-9]+(?:\.[0-9]+)?)%`)
	matches := re.FindStringSubmatch(output)
	if len(matches) >= 2 {
		p, _ := strconv.ParseFloat(matches[1], 64)
		return p
	}
	return 0
}

func CoveragePercentFromProfile(root, profilePath string, newCommand func(string, ...string) *exec.Cmd) (float64, error) {
	cmd := newCommand("go", "tool", "cover", "-func="+profilePath)
	cmd.Dir = root
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to compute coverage percent: %w", err)
	}
	return ParseCoverPercent(string(out)), nil
}

func LoadLastRunState() ui.TestRunState {
	path := filepath.Join(CoverageDir, "last_run.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return ui.TestRunState{}
	}
	var state ui.TestRunState
	if err := json.Unmarshal(data, &state); err != nil {
		return ui.TestRunState{}
	}
	return state
}

func SaveLastRunState(state ui.TestRunState) error {
	path := filepath.Join(CoverageDir, "last_run.json")
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func EnsureCoverageDir(dir string) error {
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		_ = os.Remove(dir)
	}
	return os.MkdirAll(dir, 0755)
}

func RunTests(root, coverProfile string) ([]byte, error) {
	cmd := exec.Command("go", "test", "-coverprofile="+coverProfile, "./...")
	cmd.Dir = root
	cmd.Env = os.Environ()
	return cmd.CombinedOutput()
}

func GenerateHTMLReport(root, filteredProfile, htmlReport string) error {
	cmd := exec.Command("go", "tool", "cover", "-html="+filteredProfile, "-o", htmlReport)
	cmd.Dir = root
	cmd.Env = os.Environ()
	return cmd.Run()
}


