package ui

import "testing"

func TestSplitSections(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantPre   string
		wantPlan  string
		wantCrite string
		wantLinks string
		wantTail  string
	}{
		{
			name: "empty",
			body: "",
		},
		{
			name:    "only preamble",
			body:    "Solo un parrafo introductorio.\n\nOtro renglón.",
			wantPre: "Solo un parrafo introductorio.\n\nOtro renglón.",
		},
		{
			name: "preamble + criteria",
			body: `Intro inicial.

# Acceptance Criteria

- [ ] Primer criterio.
- [ ] Segundo criterio.`,
			wantPre:   "Intro inicial.",
			wantCrite: "- [ ] Primer criterio.\n- [ ] Segundo criterio.",
		},
		{
			name: "preamble + criteria + links",
			body: `Intro.

# Acceptance Criteria

- [ ] A.

# Links

- [uno](/todo/uno.md)
- [dos](/todo/dos.md)`,
			wantPre:   "Intro.",
			wantCrite: "- [ ] A.",
			wantLinks: "- [uno](/todo/uno.md)\n- [dos](/todo/dos.md)",
		},
		{
			name: "links before criteria (orden flexible)",
			body: `Intro.

# Links

- a

# Acceptance Criteria

- [ ] b`,
			wantPre:   "Intro.",
			wantCrite: "- [ ] b",
			wantLinks: "- a",
		},
		{
			name: "spanish heading variants",
			body: `Intro.

# Criterios de aceptación

- primero

# Enlaces

- x`,
			wantPre:   "Intro.",
			wantCrite: "- primero",
			wantLinks: "- x",
		},
		{
			name: "tail after criteria (subheading pushes content to tail)",
			body: `Intro.

# Acceptance Criteria

- [ ] uno

## Notas

Cierre final.`,
			wantPre:   "Intro.",
			wantCrite: "- [ ] uno",
			wantTail:  "## Notas\n\nCierre final.",
		},
		{
			name: "plan before criteria (orden plan → criteria)",
			body: `Intro.

# Plan

1. Primer paso.
2. Segundo paso.

# Acceptance Criteria

- [ ] Verificar que el plan se cumple.`,
			wantPre:   "Intro.",
			wantPlan:  "1. Primer paso.\n2. Segundo paso.",
			wantCrite: "- [ ] Verificar que el plan se cumple.",
		},
		{
			name: "plan after links (orden links → plan, plan no se pierde)",
			body: `Intro.

# Acceptance Criteria

- [ ] c

# Links

- l

# Plan

1. a`,
			wantPre:   "Intro.",
			wantPlan:  "1. a",
			wantCrite: "- [ ] c",
			wantLinks: "- l",
		},
		{
			name: "plan de trabajo alias es equivalente a plan",
			body: `Intro.

# Plan de trabajo

1. Hacer cosas.

# Acceptance Criteria

- [ ] Fin.`,
			wantPre:   "Intro.",
			wantPlan:  "1. Hacer cosas.",
			wantCrite: "- [ ] Fin.",
		},
		{
			name: "plan en el medio no duplica en tail",
			body: `Intro.

# Acceptance Criteria

- [ ] a

# Plan

1. paso

# Links

- l`,
			wantPre:   "Intro.",
			wantPlan:  "1. paso",
			wantCrite: "- [ ] a",
			wantLinks: "- l",
		},
		{
			name: "plan no aparece → field vacio, sin regresion",
			body: `Intro.

# Acceptance Criteria

- [ ] a`,
			wantPre:   "Intro.",
			wantCrite: "- [ ] a",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitSections(tc.body)
			if got.preamble != tc.wantPre {
				t.Errorf("preamble mismatch\nwant: %q\ngot:  %q", tc.wantPre, got.preamble)
			}
			if got.plan != tc.wantPlan {
				t.Errorf("plan mismatch\nwant: %q\ngot:  %q", tc.wantPlan, got.plan)
			}
			if got.criteria != tc.wantCrite {
				t.Errorf("criteria mismatch\nwant: %q\ngot:  %q", tc.wantCrite, got.criteria)
			}
			if got.links != tc.wantLinks {
				t.Errorf("links mismatch\nwant: %q\ngot:  %q", tc.wantLinks, got.links)
			}
			if got.tail != tc.wantTail {
				t.Errorf("tail mismatch\nwant: %q\ngot:  %q", tc.wantTail, got.tail)
			}
		})
	}
}
