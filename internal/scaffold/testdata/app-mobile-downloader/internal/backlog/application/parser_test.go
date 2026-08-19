package application

import (
	"strings"
	"testing"
)

func TestParseCardFile_OK(t *testing.T) {
	raw := []byte(`---
type: backlog/card
title: Refactorizar el parser
description: Migrar de regex a YAML.
status: in_progress
priority: P1
timestamp: 2025-01-15T10:00:00Z
source: agent
agent_session: sess-abc
tags: [ui, refactor]
---

# Refactorizar el parser

Cuerpo libre.
`)

	res, err := ParseCardFile("/x/backlog/in_progress/refactorizar.md", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := res.Card

	if c.Type != "backlog/card" {
		t.Errorf("Type = %q, want backlog/card", c.Type)
	}
	if c.Title != "Refactorizar el parser" {
		t.Errorf("Title = %q", c.Title)
	}
	if c.Description != "Migrar de regex a YAML." {
		t.Errorf("Description = %q", c.Description)
	}
	if c.Status != StatusInProgress {
		t.Errorf("Status = %q", c.Status)
	}
	if c.Priority != PriorityP1 {
		t.Errorf("Priority = %q", c.Priority)
	}
	if c.Source != "agent" || c.AgentSession != "sess-abc" {
		t.Errorf("Source/AgentSession = %q/%q", c.Source, c.AgentSession)
	}
	if c.Slug != "refactorizar" {
		t.Errorf("Slug = %q", c.Slug)
	}
	if !strings.Contains(c.Body, "Cuerpo libre.") {
		t.Errorf("Body = %q", c.Body)
	}
	// timestamp y tags deben sobrevivir
	if res.Frontmatter["timestamp"] != "2025-01-15T10:00:00Z" {
		t.Errorf("FM timestamp = %v", res.Frontmatter["timestamp"])
	}
}

func TestParseCardFile_Defaults(t *testing.T) {
	raw := []byte(`---
type: backlog/card
title: Solo título
status: todo
---

body
`)
	res, err := ParseCardFile("/x/backlog/todo/solo-titulo.md", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Card.Priority != PriorityP3 {
		t.Errorf("default priority = %q, want P3", res.Card.Priority)
	}
	if res.Card.Source != "user" {
		t.Errorf("default source = %q, want user", res.Card.Source)
	}
	if res.Card.Timestamp != "" {
		t.Errorf("missing timestamp should remain empty in Card, got %q", res.Card.Timestamp)
	}
}

func TestParseCardFile_PreservesUnknownKeys(t *testing.T) {
	raw := []byte(`---
type: backlog/card
title: Tarea X
status: todo
priority: P2
custom_field: valor arbitrario
otra_key:
  nested: yes
  list: [a, b]
---

body
`)
	res, err := ParseCardFile("/x/tarea-x.md", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Frontmatter["custom_field"] != "valor arbitrario" {
		t.Errorf("custom_field lost: %v", res.Frontmatter["custom_field"])
	}
	otra, ok := res.Frontmatter["otra_key"].(map[string]any)
	if !ok {
		t.Fatalf("otra_key lost or wrong type: %T", res.Frontmatter["otra_key"])
	}
	if otra["nested"] != "yes" {
		t.Errorf("nested lost: %v", otra["nested"])
	}
}

func TestParseCardFile_NoFrontmatter(t *testing.T) {
	raw := []byte("solo markdown sin frontmatter\n")
	if _, err := ParseCardFile("/x/y.md", raw); err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

func TestParseCardFile_UnclosedFrontmatter(t *testing.T) {
	raw := []byte("---\ntype: backlog/card\ntitle: x\nstatus: todo\n")
	if _, err := ParseCardFile("/x/y.md", raw); err == nil {
		t.Fatal("expected error for unclosed frontmatter")
	}
}

func TestSplitFrontmatter(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantFM  string
		wantBody string
		wantErr bool
	}{
		{
			name: "simple",
			raw:  "---\ntitle: x\nstatus: todo\n---\n# hola",
			wantFM: "title: x\nstatus: todo",
			wantBody: "# hola",
		},
		{
			name: "body with leading newline",
			raw:  "---\ntitle: x\n---\n\n# hola",
			wantFM:  "title: x",
			wantBody: "# hola",
		},
		{
			name: "no opening",
			raw:   "title: x\n---\n",
			wantErr: true,
		},
		{
			name: "no closing",
			raw:   "---\ntitle: x\n",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm, body, err := splitFrontmatter([]byte(tc.raw))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if string(fm) != tc.wantFM {
				t.Errorf("fm = %q, want %q", fm, tc.wantFM)
			}
			if string(body) != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}
