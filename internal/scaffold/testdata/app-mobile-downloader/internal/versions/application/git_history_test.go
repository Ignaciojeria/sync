package versionsapp

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// initRepo crea un repo git temporal con la estructura mínima para
// probar el parser sin depender del repo del proyecto. Usamos
// t.TempDir para que Go limpie solo al final del test.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Tester")
	// commit inicial en main
	run("commit", "--allow-empty", "-q", "-m", "initial")
	// rama feature + 2 commits
	run("checkout", "-q", "-b", "feature/a")
	run("commit", "--allow-empty", "-q", "-m", "feature work")
	run("commit", "--allow-empty", "-q", "-m", "more feature work")
	// merge a main
	run("checkout", "-q", "main")
	run("merge", "--no-ff", "-q", "-m", "Merge branch 'feature/a'", "feature/a")
	return dir
}

func TestGitHistoryListReturnsMerges(t *testing.T) {
	dir := initRepo(t)
	g := New(dir)
	versions, err := g.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 merge, got %d", len(versions))
	}
	v := versions[0]
	if v.Branch != "feature/a" {
		t.Fatalf("expected branch feature/a, got %q", v.Branch)
	}
	if !strings.Contains(v.Message, "Merge branch 'feature/a'") {
		t.Fatalf("expected merge message, got %q", v.Message)
	}
	if !v.IsCurrent {
		t.Fatalf("expected IsCurrent=true (HEAD is the merge)")
	}
	if v.When.IsZero() {
		t.Fatalf("expected When to be parsed")
	}
	if time.Since(v.When) > time.Minute {
		t.Fatalf("When should be recent, got %v", v.When)
	}
}

func TestGitHistoryListEmptyWhenNoMerges(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t.com")
	run("config", "user.name", "T")
	run("commit", "--allow-empty", "-q", "-m", "no merges")

	g := New(dir)
	versions, err := g.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("expected 0 merges, got %d", len(versions))
	}
}

func TestGitHistoryGetReturnsVersion(t *testing.T) {
	dir := initRepo(t)
	g := New(dir)
	list, err := g.List(context.Background(), 1)
	if err != nil || len(list) == 0 {
		t.Fatalf("List setup: err=%v len=%d", err, len(list))
	}
	sha := list[0].ShortSHA
	got, err := g.Get(context.Background(), sha)
	if err != nil {
		t.Fatalf("Get(%q): %v", sha, err)
	}
	if got.ShortSHA != sha {
		t.Fatalf("expected short sha %q, got %q", sha, got.ShortSHA)
	}
}

func TestGitHistoryGetInvalidSHA(t *testing.T) {
	dir := initRepo(t)
	g := New(dir)
	if _, err := g.Get(context.Background(), "deadbeef0000"); err == nil {
		t.Fatalf("expected error for invalid sha")
	}
}

func TestParseVersionInvalidLine(t *testing.T) {
	if _, ok := parseVersion(""); ok {
		t.Fatalf("expected ok=false for empty line")
	}
	if _, ok := parseVersion("a\x1fb"); ok {
		t.Fatalf("expected ok=false for short line")
	}
}

func TestParseNumstatBinary(t *testing.T) {
	in := "10\t5\tmain.go\n-\t-\tlogo.png\n3\t0\tREADME.md\n"
	files := parseNumstat(in)
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}
	if files[0].Path != "main.go" || files[0].Adds != 10 || files[0].Dels != 5 {
		t.Fatalf("main.go mal parseado: %+v", files[0])
	}
	if !files[1].Binary || files[1].Path != "logo.png" {
		t.Fatalf("logo.png debería ser binario: %+v", files[1])
	}
	if files[2].Adds != 3 {
		t.Fatalf("README.md adds esperado 3, got %d", files[2].Adds)
	}
}