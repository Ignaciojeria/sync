package ui

import (
	"testing"

	agentapp "fixtests1/internal/agent/application"
)

func TestActiveSessionID(t *testing.T) {
	cases := []struct {
		name string
		in   PageState
		want string
	}{
		{"explicit id wins", PageState{ActiveSessionID: "s-1"}, "s-1"},
		{"fallback to ActiveSession", PageState{ActiveSession: &agentapp.Session{ID: "s-2"}}, "s-2"},
		{"nothing", PageState{}, ""},
		{"empty active falls through", PageState{ActiveSessionID: "  "}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := activeSessionID(c.in); got != c.want {
				t.Errorf("activeSessionID() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestShellSessionCWD(t *testing.T) {
	if got := shellSessionCWD(""); got != "cwd pendiente" {
		t.Errorf("empty = %q", got)
	}
	if got := shellSessionCWD("  /tmp  "); got != "/tmp" {
		t.Errorf("trimmed = %q", got)
	}
}

func TestShellTitle(t *testing.T) {
	if got := shellTitle(PageState{}); got != "sync4.run" {
		t.Errorf("default = %q", got)
	}
	if got := shellTitle(PageState{ActiveSession: &agentapp.Session{}}); got != "sync4.run" {
		t.Errorf("empty title = %q", got)
	}
	if got := shellTitle(PageState{ActiveSession: &agentapp.Session{Title: "mi chat"}}); got != "mi chat" {
		t.Errorf("titled = %q", got)
	}
}

func TestShellModel(t *testing.T) {
	if got := shellModel(PageState{}); got != "default" {
		t.Errorf("empty default = %q", got)
	}
	if got := shellModel(PageState{DefaultModel: "claude-x"}); got != "claude-x" {
		t.Errorf("default model = %q", got)
	}
	if got := shellModel(PageState{ActiveSession: &agentapp.Session{Model: "haiku"}, DefaultModel: "claude-x"}); got != "haiku" {
		t.Errorf("active overrides default = %q", got)
	}
}

func TestShellDir(t *testing.T) {
	if got := shellDir(PageState{}); got != "." {
		t.Errorf("empty = %q", got)
	}
	if got := shellDir(PageState{DefaultCWD: "/var/repo"}); got != "/var/repo" {
		t.Errorf("default cwd = %q", got)
	}
	if got := shellDir(PageState{ActiveSession: &agentapp.Session{CWD: "/active"}, DefaultCWD: "/default"}); got != "/active" {
		t.Errorf("active cwd = %q", got)
	}
}

func TestAppPath(t *testing.T) {
	if got := appPath(PageState{}, "/foo"); got != "/foo" {
		t.Errorf("no prefix /foo = %q", got)
	}
	if got := appPath(PageState{MountPrefix: "/agent"}, "/foo"); got != "/agent/foo" {
		t.Errorf("with prefix /foo = %q", got)
	}
	if got := appPath(PageState{MountPrefix: "/agent/"}, "/"); got != "/agent/" {
		t.Errorf("root with prefix = %q", got)
	}
	if got := appPath(PageState{MountPrefix: "/agent"}, "foo"); got != "/agent/foo" {
		t.Errorf("relative path = %q", got)
	}
	if got := appPath(PageState{MountPrefix: "  "}, "/foo"); got != "/foo" {
		t.Errorf("whitespace prefix = %q", got)
	}
}

func TestPreviewPathForSessionID(t *testing.T) {
	if got := previewPathForSessionID(PageState{}, "  s-1  "); got != "/agent/sessions/s-1/preview/" {
		t.Errorf("with prefix = %q", got)
	}
	// No prefix → path still gets a leading slash.
	if got := previewPathForSessionID(PageState{}, ""); got != "" {
		t.Errorf("empty id = %q", got)
	}
}

func TestShellPreviewPath(t *testing.T) {
	if got := shellPreviewPath(PageState{MountPrefix: "/agent/sessions/s-1/preview"}); got != "" {
		t.Errorf("mounted preview = %q, want empty", got)
	}
	if got := shellPreviewPath(PageState{ActiveSessionID: "s-2"}); got != "/agent/sessions/s-2/preview/" {
		t.Errorf("not mounted = %q", got)
	}
}

func TestHostAgentSessionURL(t *testing.T) {
	if got := hostAgentSessionURL(PageState{}); got != "/agent" {
		t.Errorf("empty = %q", got)
	}
	if got := hostAgentSessionURL(PageState{ActiveSessionID: "s-9"}); got != "/agent?session=s-9" {
		t.Errorf("with session = %q", got)
	}
}
