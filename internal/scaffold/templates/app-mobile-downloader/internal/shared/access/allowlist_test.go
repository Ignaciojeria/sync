package access

import "testing"

func TestIsAllowedAppEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{
			name:  "allowed email",
			email: "ignaciovl.j@gmail.com",
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
			email: "IgnacioVL.J@Gmail.com",
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
	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{
			name:  "allowed editor",
			email: "ignaciovl.j@gmail.com",
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
			email: "  IgnacioVL.J@Gmail.com  ",
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
	if !IsAllowedAnyEmail("ignaciovl.j@gmail.com") {
		t.Error("expected ignaciovl.j@gmail.com to be allowed as any email")
	}
	if IsAllowedAnyEmail("unknown@example.com") {
		t.Error("expected unknown@example.com to not be allowed")
	}
}
