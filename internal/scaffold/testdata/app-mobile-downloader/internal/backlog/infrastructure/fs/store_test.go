package fs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitinittest5/internal/backlog/application"
)

func TestStore_NewStore_CreatesColumns(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	for _, s := range application.ColumnOrder {
		path := filepath.Join(store.Root(), string(s))
		if !dirExists(path) {
			t.Errorf("column dir missing: %s", path)
		}
	}
}

// seedCard escribe un archivo .md de tarjeta directamente al bundle,
// igual que haría el agente. Devuelve el slug derivado del título.
func seedCard(t *testing.T, store *Store, status application.Status, title string, priority application.Priority) string {
	t.Helper()
	slug := application.Slugify(title, map[string]bool{})
	path := filepath.Join(store.Root(), string(status), slug+".md")
	body := "---\ntype: backlog/card\ntitle: " + title + "\nstatus: " + string(status) + "\npriority: " + string(priority) + "\nsource: agent\n---\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	store.invalidate()
	return slug
}

func TestStore_CreateAndRead(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	card, err := store.Create(context.Background(), application.StatusTodo, application.Card{
		Title:       "Crear test",
		Description: "desc",
		Status:      application.StatusTodo,
		Priority:    application.PriorityP1,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if card.Slug != "crear-test" {
		t.Errorf("Slug = %q", card.Slug)
	}

	got, err := store.Get(context.Background(), "crear-test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "Crear test" || got.Priority != application.PriorityP1 {
		t.Errorf("got = %+v", got)
	}
}

func TestStore_Move(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)
	c, _ := store.Create(context.Background(), application.StatusBacklog, application.Card{
		Title: "Moveme", Status: application.StatusBacklog, Priority: application.PriorityP2,
	})

	moved, err := store.Move(context.Background(), c.Slug, application.StatusInProgress)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if moved.Status != application.StatusInProgress {
		t.Errorf("Status = %q", moved.Status)
	}

	// List y verifica que está en in_progress, no en backlog.
	cards, _ := store.List(context.Background())
	for _, card := range cards {
		if card.Slug == c.Slug && card.Status != application.StatusInProgress {
			t.Errorf("after move: Status = %q", card.Status)
		}
	}
}

func TestStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)
	slug := seedCard(t, store, application.StatusBacklog, "Bye", application.PriorityP3)
	if err := store.Delete(context.Background(), slug); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(context.Background(), slug); err == nil {
		t.Fatal("expected ErrNotFound")
	}
}

func TestStore_Update_PreservesUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	// Crear archivo con frontmatter custom a mano. El slug del
	// filename debe matchear el Slugify del title para evitar rename
	// (que ya se cubre en TestStore_Move).
	path := filepath.Join(store.Root(), "todo", "original.md")
	raw := []byte("---\ntype: backlog/card\ntitle: Original\nstatus: todo\npriority: P2\ncustom_field: preservar\n---\nbody\n")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Update sin cambiar el slug (mismo title) → no hay rename.
	updated, err := store.Update(context.Background(), "original", application.Card{
		Title: "Original", Status: application.StatusTodo, Priority: application.PriorityP0,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Priority != application.PriorityP0 {
		t.Errorf("Priority = %q", updated.Priority)
	}

	// Releer el archivo en disco y verificar que custom_field sigue ahí.
	raw2, _ := os.ReadFile(filepath.Join(store.Root(), "todo", "original.md"))
	t.Logf("after update:\n%s", raw2)
	if !strings.Contains(string(raw2), "custom_field:") {
		t.Errorf("custom_field lost:\n%s", raw2)
	}
	if !strings.Contains(string(raw2), "priority: P0") {
		t.Errorf("priority not updated:\n%s", raw2)
	}
}

func TestStore_Board_IgnoresNonBacklogCards(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)
	if err := os.WriteFile(filepath.Join(store.Root(), "todo", "nota.md"), []byte("---\ntype: note\ntitle: nota\nstatus: todo\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root(), "todo", "card.md"), []byte("---\ntype: backlog/card\ntitle: card\nstatus: todo\npriority: P1\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	board, _, err := store.Board(context.Background())
	if err != nil {
		t.Fatalf("Board: %v", err)
	}
	if board.Count != 1 {
		t.Errorf("Count = %d", board.Count)
	}
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
