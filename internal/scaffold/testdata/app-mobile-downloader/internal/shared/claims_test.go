package shared

import "testing"

func TestFirstStringClaim(t *testing.T) {
	t.Run("returns first non-empty trimmed string", func(t *testing.T) {
		claims := map[string]any{
			"sub":   "   ",
			"email": "  user@example.com  ",
			"name":  "Ignored",
		}

		got := FirstStringClaim(claims, "sub", "email", "name")
		if got != "user@example.com" {
			t.Fatalf("FirstStringClaim() = %q, want %q", got, "user@example.com")
		}
	})

	t.Run("skips non-string values and missing keys", func(t *testing.T) {
		claims := map[string]any{
			"email": 123,
		}

		got := FirstStringClaim(claims, "missing", "email")
		if got != "" {
			t.Fatalf("FirstStringClaim() = %q, want empty", got)
		}
	})

	t.Run("nil map returns empty", func(t *testing.T) {
		got := FirstStringClaim(nil, "email")
		if got != "" {
			t.Fatalf("FirstStringClaim() = %q, want empty", got)
		}
	})

	t.Run("no keys returns empty", func(t *testing.T) {
		claims := map[string]any{"email": "user@example.com"}
		got := FirstStringClaim(claims)
		if got != "" {
			t.Fatalf("FirstStringClaim() = %q, want empty", got)
		}
	})
}

func TestFirstNonEmpty(t *testing.T) {
	t.Run("returns first non-empty trimmed value", func(t *testing.T) {
		got := FirstNonEmpty("   ", "  alpha  ", "beta")
		if got != "alpha" {
			t.Fatalf("FirstNonEmpty() = %q, want %q", got, "alpha")
		}
	})

	t.Run("returns empty when all values are blank", func(t *testing.T) {
		got := FirstNonEmpty("", "   ", "\t")
		if got != "" {
			t.Fatalf("FirstNonEmpty() = %q, want empty", got)
		}
	})

	t.Run("returns empty when no values", func(t *testing.T) {
		got := FirstNonEmpty()
		if got != "" {
			t.Fatalf("FirstNonEmpty() = %q, want empty", got)
		}
	})
}
