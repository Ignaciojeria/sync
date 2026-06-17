package access

import "testing"

func TestIsAllowedAppEmail(t *testing.T) {
	reset := withAllowedEmails(
		map[string]struct{}{"allowed@example.com": {}},
		map[string]struct{}{},
	)
	defer reset()

	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{
			name:  "allowed email",
			email: "allowed@example.com",
			want:  true,
		},
		{
			name:  "not allowed email",
			email: "other@example.com",
			want:  false,
		},
		{
			name:  "empty email",
			email: "",
			want:  false,
		},
		{
			name:  "case insensitive",
			email: "Allowed@Example.com",
			want:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAllowedAppEmail(tt.email); got != tt.want {
				t.Errorf("IsAllowedAppEmail(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

func TestIsAllowedEditorEmail(t *testing.T) {
	reset := withAllowedEmails(
		map[string]struct{}{},
		map[string]struct{}{"editor@example.com": {}},
	)
	defer reset()

	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{
			name:  "allowed editor",
			email: "editor@example.com",
			want:  true,
		},
		{
			name:  "not allowed",
			email: "other@example.com",
			want:  false,
		},
		{
			name:  "empty email",
			email: "",
			want:  false,
		},
		{
			name:  "trimmed and case insensitive",
			email: "  Editor@Example.com  ",
			want:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAllowedEditorEmail(tt.email); got != tt.want {
				t.Errorf("IsAllowedEditorEmail(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

func TestIsAllowedAnyEmail(t *testing.T) {
	reset := withAllowedEmails(
		map[string]struct{}{"app@example.com": {}},
		map[string]struct{}{"editor@example.com": {}},
	)
	defer reset()

	if !IsAllowedAnyEmail("app@example.com") {
		t.Error("expected app@example.com to be allowed as any email")
	}
	if !IsAllowedAnyEmail("editor@example.com") {
		t.Error("expected editor@example.com to be allowed as any email")
	}
	if IsAllowedAnyEmail("unknown@example.com") {
		t.Error("expected unknown@example.com to not be allowed")
	}
}

func withAllowedEmails(appEmails, editorEmails map[string]struct{}) func() {
	originalAppEmails := allowedAppEmails
	originalEditorEmails := allowedEditorEmails
	allowedAppEmails = appEmails
	allowedEditorEmails = editorEmails
	return func() {
		allowedAppEmails = originalAppEmails
		allowedEditorEmails = originalEditorEmails
	}
}
