package design

import "testing"

func TestLoadCatalog(t *testing.T) {
	themes, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	if len(themes) < 3 {
		t.Fatalf("len(themes) = %d, want >= 3", len(themes))
	}
	if themes[0].ID == "" {
		t.Fatal("themes[0].ID is empty")
	}

	ids := map[string]bool{}
	for _, theme := range themes {
		ids[theme.ID] = true
	}
	for _, want := range []string{"forest", "ocean", "sunset"} {
		if !ids[want] {
			t.Fatalf("catalog missing theme %q", want)
		}
	}
}
