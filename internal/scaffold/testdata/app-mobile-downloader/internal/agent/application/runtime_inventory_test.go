package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func setProcRoot(t *testing.T, dir string) {
	t.Helper()
	prev := procRoot
	procRoot = dir
	t.Cleanup(func() { procRoot = prev })
}

func setResolveCwd(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	prev := resolveCwd
	resolveCwd = fn
	t.Cleanup(func() { resolveCwd = prev })
}

func writeComm(t *testing.T, root string, pid int, comm string) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "comm"), []byte(comm+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
}

func writeProcFile(t *testing.T, root string, pid int, name, body string) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o640); err != nil {
		t.Fatal(err)
	}
}

// statLineWithStart arma una línea /proc/<pid>/stat de >=22 campos
// donde el campo 22 (índice 21) vale startJiffies. Los primeros 21
// son dummy "0" porque la implementación sólo lee ese índice.
func statLineWithStart(startJiffies string) string {
	fields := make([]string, 22)
	for i := 0; i < 21; i++ {
		fields[i] = "0"
	}
	fields[21] = startJiffies
	return strings.Join(fields, " ")
}

func TestFormatRuntimeElapsed_Branches(t *testing.T) {
	cases := []struct {
		secs int64
		want string
	}{
		{0, "0s"},
		{1, "1s"},
		{59, "59s"},
		{60, "1m00s"},
		{125, "2m05s"},
		{3599, "59m59s"},
		{3600, "1h00m"},
		{3725, "1h02m"},
		{7325, "2h02m"},
	}
	for _, c := range cases {
		if got := formatRuntimeElapsed(c.secs); got != c.want {
			t.Errorf("formatRuntimeElapsed(%d) = %q, want %q", c.secs, got, c.want)
		}
	}
}

func TestListAgentRuntimes_ProcRootMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-existe")
	setProcRoot(t, missing)
	rts, err := ListAgentRuntimes(context.Background())
	if err == nil {
		t.Fatalf("expected error for missing procRoot, got rts=%+v", rts)
	}
	if rts != nil {
		t.Fatalf("expected nil result on error, got %+v", rts)
	}
	if !strings.Contains(err.Error(), "read /proc") {
		t.Fatalf("expected wrapped read /proc error, got %v", err)
	}
}

func TestListAgentRuntimes_NoEntries(t *testing.T) {
	setProcRoot(t, t.TempDir())
	rts, err := ListAgentRuntimes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rts) != 0 {
		t.Fatalf("expected empty list, got %+v", rts)
	}
}

func TestListAgentRuntimes_SkipsNonDirectoriesAndBadNames(t *testing.T) {
	root := t.TempDir()
	setProcRoot(t, root)
	if err := os.WriteFile(filepath.Join(root, "not-a-dir"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"self", "thread-self", "0", "-5", "abc"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	rts, err := ListAgentRuntimes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rts) != 0 {
		t.Fatalf("expected no runtimes, got %+v", rts)
	}
}

func TestListAgentRuntimes_SkipsNonPiComm(t *testing.T) {
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 100, "node")
	writeComm(t, root, 200, "bash")
	rts, err := ListAgentRuntimes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rts) != 0 {
		t.Fatalf("expected no runtimes, got %+v", rts)
	}
}

func TestListAgentRuntimes_SkipsCommReadError(t *testing.T) {
	root := t.TempDir()
	setProcRoot(t, root)
	if err := os.MkdirAll(filepath.Join(root, "999"), 0o750); err != nil {
		t.Fatal(err)
	}
	rts, err := ListAgentRuntimes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rts) != 0 {
		t.Fatalf("expected no runtimes when comm is missing, got %+v", rts)
	}
}

func TestListAgentRuntimes_PiCommWithoutProcFiles(t *testing.T) {
	// comm="pi" pero sin stat/status → readRuntimeInfo devuelve ok=false
	// (proceso murió en medio del scan) → se filtra. Sólo queda el
	// caso donde al menos uno de los dos archivos existe.
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 500, "pi")
	// stat presente sin starttime parseable → hasData=true → ok=true
	writeProcFile(t, root, 500, "stat", "500 (pi) S 1 500 500")
	setResolveCwd(t, func(string) (string, error) { return "", errors.New("no cwd") })

	rts, err := ListAgentRuntimes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rts) != 1 {
		t.Fatalf("expected one runtime (stat present), got %+v", rts)
	}
	rt := rts[0]
	if rt.PID != 500 || rt.Command != "pi" {
		t.Fatalf("unexpected runtime: %+v", rt)
	}
	if rt.RSSKB != 0 || rt.CWD != "" || rt.Owner != "" {
		t.Fatalf("expected empty fields, got %+v", rt)
	}
}

func TestListAgentRuntimes_PiCommTrulyEmpty(t *testing.T) {
	// comm="pi" sin stat ni status → hasData=false → readRuntimeInfo
	// devuelve ok=false → el entry se filtra del output.
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 600, "pi")
	setResolveCwd(t, func(string) (string, error) { return "", errors.New("no cwd") })

	rts, err := ListAgentRuntimes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rts) != 0 {
		t.Fatalf("expected empty (proc with no stat/status), got %+v", rts)
	}
}

func TestReadRuntimeInfo_MissingCwdAndFiles(t *testing.T) {
	// Sin stat ni status → hasData=false → ok=false.
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 42, "pi")
	setResolveCwd(t, func(string) (string, error) { return "", errors.New("no cwd") })
	rt, ok := readRuntimeInfo(filepath.Join(root, "42"), 42)
	if ok {
		t.Fatalf("expected ok=false, got %+v", rt)
	}
	if rt.PID != 42 || rt.Command != "pi" {
		t.Fatalf("unexpected rt: %+v", rt)
	}
}

func TestReadRuntimeInfo_OnlyStatus(t *testing.T) {
	// Sin stat pero con status → hasData=true, Elapsed queda vacío.
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 43, "pi")
	writeProcFile(t, root, 43, "status", "Name:\tpi\nVmRSS:\t 2048 kB\n")
	setResolveCwd(t, func(string) (string, error) { return "", errors.New("no") })
	rt, ok := readRuntimeInfo(filepath.Join(root, "43"), 43)
	if !ok {
		t.Fatal("ok=false")
	}
	if rt.RSSKB != 2048 {
		t.Errorf("RSSKB = %d, want 2048", rt.RSSKB)
	}
	if rt.Elapsed != "" {
		t.Errorf("Elapsed = %q, want empty", rt.Elapsed)
	}
}

func TestReadRuntimeInfo_CwdSetAndOwnerInferred(t *testing.T) {
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 80, "pi")
	// CWD cae bajo /tmp/agent-work/<id>/ → debe inferir Owner=<id>.
	setResolveCwd(t, func(string) (string, error) { return "/tmp/agent-work/sess-x", nil })
	writeProcFile(t, root, 80, "stat", statLineWithStart("0"))
	writeProcFile(t, root, 80, "status", "Name:\tpi\nVmRSS:\t 4096 kB\n")
	if err := os.WriteFile(filepath.Join(root, "uptime"), []byte("0.0 0\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	rt, ok := readRuntimeInfo(filepath.Join(root, "80"), 80)
	if !ok {
		t.Fatal("ok=false")
	}
	if rt.CWD != "/tmp/agent-work/sess-x" {
		t.Errorf("CWD = %q", rt.CWD)
	}
	if rt.Owner != "sess-x" {
		t.Errorf("Owner = %q, want sess-x", rt.Owner)
	}
	if rt.RSSKB != 4096 {
		t.Errorf("RSSKB = %d, want 4096", rt.RSSKB)
	}
}

func TestReadRuntimeInfo_OwnerMarkerNoTrailingSlash(t *testing.T) {
	// CWD exactamente "/tmp/agent-work/<id>" sin slash final → Owner=<id>.
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 82, "pi")
	setResolveCwd(t, func(string) (string, error) { return "/tmp/agent-work/alone", nil })
	writeProcFile(t, root, 82, "status", "Name:\tpi\nVmRSS:\t 1 kB\n")
	rt, ok := readRuntimeInfo(filepath.Join(root, "82"), 82)
	if !ok || rt.Owner != "alone" {
		t.Fatalf("rt=%+v ok=%v", rt, ok)
	}
}

func TestReadRuntimeInfo_CwdNotUnderAgentWork(t *testing.T) {
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 81, "pi")
	// CWD fuera de /tmp/agent-work/ → Owner queda vacío (slash >= 0
	// pero el resto antes del slash es "sess-b" → no matchea).
	// Para ejercitar el branch del slash, usamos una CWD que SÍ
	// matchea el marker.
	setResolveCwd(t, func(string) (string, error) { return "/etc/something/else", nil })
	writeProcFile(t, root, 81, "status", "Name:\tpi\nVmRSS:\t 1 kB\n")
	rt, ok := readRuntimeInfo(filepath.Join(root, "81"), 81)
	if !ok || rt.CWD != "/etc/something/else" || rt.Owner != "" {
		t.Fatalf("rt=%+v ok=%v", rt, ok)
	}
}

func TestReadRuntimeInfo_StatTooShort(t *testing.T) {
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 50, "pi")
	if err := os.WriteFile(filepath.Join(root, "uptime"), []byte("1234.5 0\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	writeProcFile(t, root, 50, "stat", "50 (pi) S 1 50 50")
	writeProcFile(t, root, 50, "status", "Name:\tpi\n") // para hasData=true
	setResolveCwd(t, func(string) (string, error) { return "", errors.New("no") })
	rt, ok := readRuntimeInfo(filepath.Join(root, "50"), 50)
	if !ok {
		t.Fatal("ok=false")
	}
	if rt.Elapsed != "" {
		t.Fatalf("expected empty elapsed for short stat, got %q", rt.Elapsed)
	}
}

func TestReadRuntimeInfo_StatStartJiffiesUnparseable(t *testing.T) {
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 60, "pi")
	if err := os.WriteFile(filepath.Join(root, "uptime"), []byte("1234.5 0\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	writeProcFile(t, root, 60, "stat", statLineWithStart("notanumber"))
	writeProcFile(t, root, 60, "status", "Name:\tpi\n")
	setResolveCwd(t, func(string) (string, error) { return "", errors.New("no") })
	rt, ok := readRuntimeInfo(filepath.Join(root, "60"), 60)
	if !ok || rt.Elapsed != "" {
		t.Fatalf("expected empty elapsed, got rt=%+v ok=%v", rt, ok)
	}
}

func TestReadRuntimeInfo_UptimeMissing(t *testing.T) {
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 70, "pi")
	writeProcFile(t, root, 70, "stat", statLineWithStart("100"))
	writeProcFile(t, root, 70, "status", "Name:\tpi\n")
	// Sin archivo uptime: la rama errU != nil → Elapsed queda vacío.
	setResolveCwd(t, func(string) (string, error) { return "", errors.New("no") })
	rt, ok := readRuntimeInfo(filepath.Join(root, "70"), 70)
	if !ok || rt.Elapsed != "" {
		t.Fatalf("expected empty elapsed when uptime missing, got rt=%+v ok=%v", rt, ok)
	}
}

func TestReadRuntimeInfo_UptimeMalformed(t *testing.T) {
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 71, "pi")
	if err := os.WriteFile(filepath.Join(root, "uptime"), []byte("notanumber 0\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	writeProcFile(t, root, 71, "stat", statLineWithStart("500"))
	writeProcFile(t, root, 71, "status", "Name:\tpi\n")
	setResolveCwd(t, func(string) (string, error) { return "", errors.New("no") })
	rt, ok := readRuntimeInfo(filepath.Join(root, "71"), 71)
	if !ok || rt.Elapsed != "" {
		t.Fatalf("expected empty elapsed when uptime malformed, got rt=%+v ok=%v", rt, ok)
	}
}

func TestReadRuntimeInfo_ElapsedNegativeClamped(t *testing.T) {
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 72, "pi")
	// uptime=1s, starttime=10000 jiffies (100s). elapsed = 1 - 100 = -99 → "0s"
	if err := os.WriteFile(filepath.Join(root, "uptime"), []byte("1.0 0\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	writeProcFile(t, root, 72, "stat", statLineWithStart("10000"))
	writeProcFile(t, root, 72, "status", "Name:\tpi\n")
	setResolveCwd(t, func(string) (string, error) { return "", errors.New("no") })
	rt, ok := readRuntimeInfo(filepath.Join(root, "72"), 72)
	if !ok {
		t.Fatal("ok=false")
	}
	if rt.Elapsed != "0s" {
		t.Fatalf("expected Elapsed=0s (clamped), got %q", rt.Elapsed)
	}
}

func TestReadRuntimeInfo_ElapsedValidFormats(t *testing.T) {
	cases := []struct {
		name      string
		uptimeSec float64
		start     string
		want      string
	}{
		{"under-minute", 1000.0, "99900", "1s"}, // 1000 - 999 = 1
		{"minutes", 3700.0, "100000", "45m00s"}, // 3700 - 1000 = 2700s
		{"hours", 5000.0, "100000", "1h06m"},   // 5000 - 1000 = 4000s = 66m40s
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			setProcRoot(t, root)
			pid := 90 + i
			writeComm(t, root, pid, "pi")
			body := strconv.FormatFloat(c.uptimeSec, 'f', 1, 64) + " 0"
			if err := os.WriteFile(filepath.Join(root, "uptime"), []byte(body), 0o640); err != nil {
				t.Fatal(err)
			}
			writeProcFile(t, root, pid, "stat", statLineWithStart(c.start))
			setResolveCwd(t, func(string) (string, error) { return "", errors.New("no") })
			rt, ok := readRuntimeInfo(filepath.Join(root, strconv.Itoa(pid)), pid)
			if !ok {
				t.Fatal("ok=false")
			}
			if rt.Elapsed != c.want {
				t.Errorf("uptime=%v start=%s → Elapsed=%q, want %q", c.uptimeSec, c.start, rt.Elapsed, c.want)
			}
		})
	}
}

func TestReadRuntimeInfo_StatusVmRSSParseError(t *testing.T) {
	// VmRSS con valor no numérico → RSSKB queda en 0.
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 91, "pi")
	writeProcFile(t, root, 91, "status", "Name:\tpi\nVmRSS:\t notanumber kB\n")
	setResolveCwd(t, func(string) (string, error) { return "", errors.New("no") })
	rt, ok := readRuntimeInfo(filepath.Join(root, "91"), 91)
	if !ok || rt.RSSKB != 0 {
		t.Fatalf("rt=%+v ok=%v", rt, ok)
	}
}

func TestReadRuntimeInfo_StatusNoVmRSS(t *testing.T) {
	// status sin línea VmRSS → RSSKB=0 y el for-loop sale sin tocarlo.
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 92, "pi")
	writeProcFile(t, root, 92, "status", "Name:\tpi\nState:\tR\n")
	setResolveCwd(t, func(string) (string, error) { return "", errors.New("no") })
	rt, ok := readRuntimeInfo(filepath.Join(root, "92"), 92)
	if !ok || rt.RSSKB != 0 {
		t.Fatalf("rt=%+v ok=%v", rt, ok)
	}
}

func TestListAgentRuntimes_HappyPath(t *testing.T) {
	root := t.TempDir()
	setProcRoot(t, root)
	writeComm(t, root, 1000, "pi")
	writeComm(t, root, 2000, "pi")
	if err := os.WriteFile(filepath.Join(root, "uptime"), []byte("5000.0 0\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	// elapsed = uptime - start/100. PID 1000 → 5000 - 100000/100 = 4000 → "1h06m"
	writeProcFile(t, root, 1000, "stat", statLineWithStart("100000"))
	// PID 2000 → 5000 - 400000/100 = 1000 → "16m40s"
	writeProcFile(t, root, 2000, "stat", statLineWithStart("400000"))
	writeProcFile(t, root, 1000, "status", "Name:\tpi\nVmRSS:\t 12345 kB\n")
	writeProcFile(t, root, 2000, "status", "Name:\tpi\nVmRSS:\t 67890 kB\n")
	setResolveCwd(t, func(proc string) (string, error) {
		switch filepath.Base(proc) {
		case "1000":
			return "/home/user/sess-a", nil
		case "2000":
			return "/tmp/agent-work/sess-b/inner", nil
		}
		return "", errors.New("no cwd")
	})

	rts, err := ListAgentRuntimes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rts) != 2 {
		t.Fatalf("expected 2 runtimes, got %+v", rts)
	}
	byPID := map[int]ProcessInfo{}
	for _, rt := range rts {
		byPID[rt.PID] = rt
	}
	rt1000 := byPID[1000]
	if rt1000.Command != "pi" || rt1000.CWD != "/home/user/sess-a" {
		t.Errorf("PID 1000 unexpected: %+v", rt1000)
	}
	if rt1000.RSSKB != 12345 || rt1000.Owner != "" {
		t.Errorf("PID 1000 rss=%d owner=%q", rt1000.RSSKB, rt1000.Owner)
	}
	if rt1000.Elapsed != "1h06m" {
		t.Errorf("PID 1000 Elapsed = %q, want 1h06m", rt1000.Elapsed)
	}
	rt2000 := byPID[2000]
	if rt2000.Owner != "sess-b" {
		t.Errorf("PID 2000 Owner = %q, want sess-b", rt2000.Owner)
	}
	if rt2000.RSSKB != 67890 {
		t.Errorf("PID 2000 RSS = %d", rt2000.RSSKB)
	}
	if rt2000.Elapsed != "16m40s" {
		t.Errorf("PID 2000 Elapsed = %q, want 16m40s", rt2000.Elapsed)
	}
}
