package application

import (
	"strings"
	"testing"
)

func TestWriteCardFile_PreservesUnknownKeys(t *testing.T) {
	card := Card{
		Type:        "backlog/card",
		Title:       "Refactor X",
		Status:      StatusTodo,
		Priority:    PriorityP1,
		Source:      "user",
		Timestamp:   "2025-01-15T10:00:00Z",
		Body:        "# hola\n",
	}
	originalFM := map[string]any{
		"type":         "backlog/card", // debe ser reescrito, no duplicado
		"custom_field": "preservar",
		"otra_key":     map[string]any{"nested": "yes"},
		"title":        "stale title", // debe ser reescrito
	}

	out, err := WriteCardFile(card, originalFM)
	if err != nil {
		t.Fatalf("WriteCardFile: %v", err)
	}
	s := string(out)

	// custom_field y otra_key deben seguir presentes
	if !strings.Contains(s, "custom_field:") {
		t.Errorf("custom_field missing:\n%s", s)
	}
	if !strings.Contains(s, "nested:") {
		t.Errorf("nested map lost:\n%s", s)
	}
	// El title stale NO debe aparecer
	if strings.Contains(s, "stale title") {
		t.Errorf("stale title not overwritten:\n%s", s)
	}
	// type NO debe aparecer duplicado
	if strings.Count(s, "type:") != 1 {
		t.Errorf("type duplicated:\n%s", s)
	}
}

func TestWriteCardFile_DefaultsApplied(t *testing.T) {
	c := Card{
		Title:    "Sin type ni source",
		Status:   StatusBacklog,
		Priority: PriorityP2,
		Body:     "",
	}
	out, err := WriteCardFile(c, nil)
	if err != nil {
		t.Fatalf("WriteCardFile: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "type: backlog/card") {
		t.Errorf("default type missing:\n%s", s)
	}
	if !strings.Contains(s, "source: user") {
		t.Errorf("default source missing:\n%s", s)
	}
	// Sin timestamp en Card → debe completarse con NowRFC3339 (RFC3339 UTC).
	if !strings.Contains(s, "timestamp:") {
		t.Errorf("timestamp not auto-completed:\n%s", s)
	}
	// description vacía NO debe aparecer.
	if strings.Contains(s, "description:") {
		t.Errorf("empty description leaked:\n%s", s)
	}
}

func TestWriteCardFile_BodyNewline(t *testing.T) {
	c := Card{Type: "backlog/card", Title: "x", Status: StatusTodo, Body: "sin newline final"}
	out, _ := WriteCardFile(c, nil)
	if !strings.HasSuffix(string(out), "\n") {
		t.Errorf("body should end with newline:\n%q", out)
	}
}

func TestRoundtrip(t *testing.T) {
	raw := []byte(`---
type: backlog/card
title: Roundtrip
description: desc
status: todo
priority: P2
timestamp: 2025-01-15T10:00:00Z
source: user
tags: [a, b]
unknown: kept
---
# Body original
`)

	res, err := ParseCardFile("/x/y.md", raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := WriteCardFile(res.Card, res.Frontmatter)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	res2, err := ParseCardFile("/x/y.md", out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if res2.Card.Title != res.Card.Title ||
		res2.Card.Status != res.Card.Status ||
		res2.Card.Priority != res.Card.Priority {
		t.Errorf("card drifted:\nbefore=%+v\nafter=%+v", res.Card, res2.Card)
	}
	if res2.Frontmatter["unknown"] != "kept" {
		t.Errorf("unknown key lost across roundtrip: %v", res2.Frontmatter["unknown"])
	}
	if !strings.Contains(res2.Card.Body, "# Body original") {
		t.Errorf("body lost: %q", res2.Card.Body)
	}
}

func TestSlugify(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Refactor el parser", "refactor-el-parser"},
		{"  Espacios  y  cosas  ", "espacios-y-cosas"},
		{"Multi!!!Chars???Here", "multi-chars-here"},
		{"---trim---", "trim"},
		{"á é í ó ú", "a-e-i-o-u"}, // diacríticos separados por espacios
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := Slugify(tc.in, nil)
			if got != tc.want {
				t.Errorf("Slugify(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSlugify_Collision(t *testing.T) {
	used := map[string]bool{"refactor": true}
	if got := Slugify("Refactor", used); got != "refactor-2" {
		t.Errorf("first collision: got %q", got)
	}
	used["refactor-2"] = true
	if got := Slugify("Refactor", used); got != "refactor-3" {
		t.Errorf("second collision: got %q", got)
	}
}

func TestSlugify_Truncate(t *testing.T) {
	long := strings.Repeat("a", 100)
	got := Slugify(long, nil)
	if len(got) > 60 {
		t.Errorf("slug too long: %d chars", len(got))
	}
}

func TestSlugFromPath(t *testing.T) {
	if got := SlugFromPath("/abs/backlog/todo/refactor.md"); got != "refactor" {
		t.Errorf("got %q", got)
	}
	if got := SlugFromPath("rel/todo/x.md"); got != "x" {
		t.Errorf("got %q", got)
	}
}

func TestConceptID(t *testing.T) {
	got := ConceptID("/abs", "/abs/backlog/todo/refactor.md")
	if got != "backlog/todo/refactor" {
		t.Errorf("got %q", got)
	}
}
