package application

import (
	"context"
	"testing"
)

func TestService_Create(t *testing.T) {
	s := NewService(newMemStore())
	c, err := s.Create(context.Background(), "Hola mundo", "desc", StatusTodo, PriorityP1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c.Slug != "hola-mundo" {
		t.Errorf("Slug = %q", c.Slug)
	}
	if c.Status != StatusTodo || c.Priority != PriorityP1 {
		t.Errorf("card = %+v", c)
	}
	if c.Timestamp == "" {
		t.Errorf("timestamp missing")
	}
}

func TestService_Create_Defaults(t *testing.T) {
	s := NewService(newMemStore())
	// Status vacío + priority inválida → defaults.
	c, err := s.Create(context.Background(), "Solo título", "", "", "ZZ")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c.Status != StatusBacklog {
		t.Errorf("Status = %q, want backlog", c.Status)
	}
	if c.Priority != PriorityP3 {
		t.Errorf("Priority = %q, want P3", c.Priority)
	}
}

func TestService_Create_TitleRequired(t *testing.T) {
	s := NewService(newMemStore())
	_, err := s.Create(context.Background(), "   ", "x", StatusTodo, PriorityP1)
	if err == nil {
		t.Fatal("expected error on empty title")
	}
}

func TestService_Move(t *testing.T) {
	store := newMemStore()
	s := NewService(store)
	c, _ := s.Create(context.Background(), "Moverme", "", StatusBacklog, PriorityP1)

	moved, err := s.Move(context.Background(), c.Slug, StatusTodo)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if moved.Status != StatusTodo {
		t.Errorf("Status = %q", moved.Status)
	}
}

func TestService_Move_InvalidStatus(t *testing.T) {
	s := NewService(newMemStore())
	_, err := s.Move(context.Background(), "x", Status("invalid"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestService_SetPriority_Clamps(t *testing.T) {
	s := NewService(newMemStore())
	c, _ := s.Create(context.Background(), "P", "", StatusBacklog, PriorityP1)
	if _, err := s.SetPriority(context.Background(), c.Slug, PriorityP0); err != nil {
		t.Fatalf("SetPriority: %v", err)
	}
	if _, err := s.SetPriority(context.Background(), c.Slug, "P10"); err != nil {
		t.Fatalf("SetPriority over: %v", err)
	}
	got, _ := s.Get(context.Background(), c.Slug)
	if got.Priority != PriorityP3 {
		t.Errorf("clamp: Priority = %q, want P3", got.Priority)
	}
}

func TestService_Delete(t *testing.T) {
	s := NewService(newMemStore())
	c, _ := s.Create(context.Background(), "Borrar", "", StatusBacklog, PriorityP1)
	if err := s.Delete(context.Background(), c.Slug); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(context.Background(), c.Slug); err == nil {
		t.Fatal("expected ErrNotFound after delete")
	}
}

func TestService_Get_NotFound(t *testing.T) {
	s := NewService(newMemStore())
	if _, err := s.Get(context.Background(), "missing"); err == nil {
		t.Fatal("expected ErrNotFound")
	}
}

func TestService_Board(t *testing.T) {
	s := NewService(newMemStore())
	_, _ = s.Create(context.Background(), "A", "", StatusTodo, PriorityP1)
	_, _ = s.Create(context.Background(), "B", "", StatusInProgress, PriorityP0)

	board, err := s.Board(context.Background())
	if err != nil {
		t.Fatalf("Board: %v", err)
	}
	if board.Count != 2 {
		t.Errorf("Count = %d", board.Count)
	}
	if len(board.Columns) != 4 {
		t.Errorf("Columns = %d, want 4", len(board.Columns))
	}
}
