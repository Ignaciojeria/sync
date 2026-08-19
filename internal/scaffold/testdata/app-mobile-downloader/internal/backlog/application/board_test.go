package application

import (
	"testing"
)

func makeCard(slug, title string, status Status, priority Priority) Card {
	return Card{
		Type:      DefaultType,
		Title:     title,
		Status:    status,
		Priority:  priority,
		Slug:      slug,
		Timestamp: "2025-01-15T10:00:00Z",
		Source:    "user",
	}
}

func TestToBoard_FiltersNonBacklogCards(t *testing.T) {
	cards := []Card{
		makeCard("a", "Tarjeta A", StatusTodo, PriorityP1),
		{
			Type: "note", Title: "Nota suelta", Status: StatusTodo,
			Slug: "n", Timestamp: "2025-01-15T10:00:00Z",
		},
	}
	board, invalids := ToBoard(cards)
	if board.Count != 1 {
		t.Errorf("Count = %d, want 1", board.Count)
	}
	if len(invalids) != 0 {
		t.Errorf("invalids = %v", invalids)
	}
}

func TestToBoard_ReportsInvalids(t *testing.T) {
	cards := []Card{
		makeCard("ok", "OK", StatusTodo, PriorityP1),
		{Path: "/x/bad.md", Type: DefaultType, Title: "  ", Status: StatusTodo, Priority: PriorityP1, Slug: "bad", Timestamp: "x"},
	}
	board, invalids := ToBoard(cards)
	if board.Count != 1 {
		t.Errorf("Count = %d, want 1", board.Count)
	}
	if len(invalids) != 1 || invalids[0].Path != "/x/bad.md" {
		t.Errorf("invalids = %+v", invalids)
	}
}

func TestToBoard_Order(t *testing.T) {
	cards := []Card{
		{Type: DefaultType, Title: "B", Status: StatusTodo, Priority: PriorityP2, Slug: "p2-old", Timestamp: "2025-01-15T09:00:00Z", Source: "user"},
		{Type: DefaultType, Title: "A", Status: StatusTodo, Priority: PriorityP0, Slug: "p0-new", Timestamp: "2025-01-15T11:00:00Z", Source: "user"},
		{Type: DefaultType, Title: "C", Status: StatusTodo, Priority: PriorityP2, Slug: "p2-newer", Timestamp: "2025-01-15T10:00:00Z", Source: "user"},
		{Type: DefaultType, Title: "D", Status: StatusTodo, Priority: PriorityP0, Slug: "p0-old", Timestamp: "2025-01-15T08:00:00Z", Source: "user"},
	}
	board, _ := ToBoard(cards)
	if len(board.Columns) == 0 {
		t.Fatal("no columns")
	}
	todo := board.Columns[1].Cards // [0]=backlog, [1]=todo
	wantSlugs := []string{"p0-old", "p0-new", "p2-old", "p2-newer"}
	for i, want := range wantSlugs {
		if todo[i].Slug != want {
			t.Errorf("todo[%d].Slug = %q, want %q", i, todo[i].Slug, want)
		}
	}
}
