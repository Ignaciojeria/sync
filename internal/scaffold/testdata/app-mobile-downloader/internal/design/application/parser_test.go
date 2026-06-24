package design

import "testing"

func TestParseDocument(t *testing.T) {
	content := `---
version: alpha
name: Ocean
description: Tema de prueba.
colors:
  primary: "#2563eb"
spacing:
  sm: 8px
typography:
  body-md:
    fontFamily: Inter, system-ui, sans-serif
    fontSize: 16px
x-pi:
  themeId: ocean
  colorScheme: light
  daisyui:
    base-100: "{colors.primary}"
---

# Ocean

## Overview

Texto.`

	doc, err := ParseDocument(content)
	if err != nil {
		t.Fatalf("ParseDocument() error = %v", err)
	}
	if got, want := doc.Name, "Ocean"; got != want {
		t.Fatalf("doc.Name = %q, want %q", got, want)
	}
	if got, want := doc.Colors["primary"], "#2563eb"; got != want {
		t.Fatalf("doc.Colors[primary] = %q, want %q", got, want)
	}
	if got, want := doc.Spacing["sm"], "8px"; got != want {
		t.Fatalf("doc.Spacing[sm] = %q, want %q", got, want)
	}
	if got, want := doc.Typography["body-md"].FontFamily, "Inter, system-ui, sans-serif"; got != want {
		t.Fatalf("doc.Typography[body-md].FontFamily = %q, want %q", got, want)
	}
	if got, want := doc.XPi.DaisyUI["base-100"], "{colors.primary}"; got != want {
		t.Fatalf("doc.XPi.DaisyUI[base-100] = %q, want %q", got, want)
	}
	if doc.Body == "" {
		t.Fatal("doc.Body is empty")
	}
}

func TestParseDocumentMissingFrontmatter(t *testing.T) {
	_, err := ParseDocument("# no frontmatter")
	if err == nil {
		t.Fatal("ParseDocument() error = nil, want error")
	}
}
