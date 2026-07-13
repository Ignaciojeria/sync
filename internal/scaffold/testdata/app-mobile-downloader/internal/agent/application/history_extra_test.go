package application

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseHistoryBefore(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
	}{
		{"", 0},
		{"  ", 0},
		{"abc", 0},
		{"123", 123},
		{"  42  ", 42},
		{"-1", 0}, // ParseUint no acepta negativos
		{"99999999999", 99999999999},
	}
	for _, c := range cases {
		if got := ParseHistoryBefore(c.in); got != c.want {
			t.Errorf("ParseHistoryBefore(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseHistoryLimit(t *testing.T) {
	cases := []struct {
		in       string
		fallback int
		want     int
	}{
		{"", 10, 10},
		{"  ", 10, 10},
		{"abc", 10, 10},
		{"0", 10, 10},
		{"-5", 10, 10},
		{"7", 10, 7},
		{"100", 10, 50}, // clamp a 50
		{"50", 10, 50},
	}
	for _, c := range cases {
		if got := ParseHistoryLimit(c.in, c.fallback); got != c.want {
			t.Errorf("ParseHistoryLimit(%q, %d) = %d, want %d", c.in, c.fallback, got, c.want)
		}
	}
}

func TestSanitizeSessionID(t *testing.T) {
	sep := string(filepath.Separator)
	cases := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"with/slash", "with_slash"},
		{"with..dots", "with_dots"},
		{"back" + sep + "slash", "back_slash"},
	}
	for _, c := range cases {
		if got := sanitizeSessionID(c.in); got != c.want {
			t.Errorf("sanitizeSessionID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExtractMessageText(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`""`, ""},
		{`null`, ""},
		{`"text"`, "text"},
		{`42`, "42"},
		{`"  hello  "`, "  hello  "}, // ponytail: la función NO trimea el interior.
		{`{"un":"objeto"}`, `{"un":"objeto"}`},
	}
	for _, c := range cases {
		if got := extractMessageText(json.RawMessage(c.in)); got != c.want {
			t.Errorf("extractMessageText(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPreviewText(t *testing.T) {
	if got := previewText("hola", 10); got != "hola" {
		t.Errorf("under max = %q", got)
	}
	if got := previewText("  hola  ", 10); got != "hola" {
		t.Errorf("trim under max = %q", got)
	}
	got := previewText(strings.Repeat("a", 200), 50)
	if !strings.Contains(got, "…") {
		t.Errorf("missing ellipsis: %q", got)
	}
	// El trim del inicio elimina whitespace del recorte, pero conserva
	// los últimos caracteres quepan en `max`.
	if got := previewText("   "+strings.Repeat("b", 100)+"   ", 10); got == "" {
		t.Errorf("trimmed = empty")
	}
	if got := previewText("hola", 0); got != "hola" {
		t.Errorf("max<=0 returns trimmed input = %q", got)
	}
}
