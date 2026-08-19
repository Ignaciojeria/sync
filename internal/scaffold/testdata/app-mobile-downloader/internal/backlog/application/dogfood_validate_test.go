package application

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDogfoodCardsParse valida que cualquier card sembrada en
// internal/backlog/board (cualquier columna) parsea y cumple Validate.
// Sirve como smoke de "el agente no rompió el formato al editarla".
func TestDogfoodCardsParse(t *testing.T) {
	root := filepath.Join("..", "..", "board")
	dirs := []string{"backlog", "todo", "in_progress", "done"}
	for _, d := range dirs {
		matches, err := filepath.Glob(filepath.Join(root, d, "*.md"))
		if err != nil {
			t.Fatalf("glob %s: %v", d, err)
		}
		for _, p := range matches {
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("%s: read: %v", p, err)
			}
			res, err := ParseCardFile(p, raw)
			if err != nil {
				t.Fatalf("%s: parse: %v", p, err)
			}
			if err := res.Card.Validate(); err != nil {
				t.Fatalf("%s: validate: %v", p, err)
			}
			expectedStatus := Status(d)
			if expectedStatus == Status("in_progress") {
				expectedStatus = StatusInProgress
			}
			if d == "backlog" {
				expectedStatus = StatusBacklog
			}
			if d == "todo" {
				expectedStatus = StatusTodo
			}
			if d == "done" {
				expectedStatus = StatusDone
			}
			if res.Card.Status != expectedStatus {
				t.Errorf("%s: status=%q but dir=%s (expected %q)",
					p, res.Card.Status, d, expectedStatus)
			}
			t.Logf("OK  %s  [%s / %s] %q",
				res.Card.Slug, res.Card.Status, res.Card.Priority, res.Card.Title)
		}
	}
}
