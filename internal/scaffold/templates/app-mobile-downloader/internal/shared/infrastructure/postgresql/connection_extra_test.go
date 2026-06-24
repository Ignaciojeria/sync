package postgresql

import "testing"

func TestParseDatabaseNameInvalidURL(t *testing.T) {
	// url.Parse returns an error for these inputs; parseDatabaseName must propagate it.
	inputs := []string{
		"%",
		"http://[",
		string([]byte{0x7f}),
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			got, err := parseDatabaseName(in)
			if err == nil {
				t.Fatalf("expected error for %q, got %q", in, got)
			}
		})
	}
}
