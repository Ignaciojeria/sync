package application

import (
	"sort"
)

// Board es la vista agregada del tablero: una columna por Status
// en ColumnOrder, con las tarjetas ordenadas según el SPEC §10.
//
// El Board NO es persistente: se reconstruye en cada request
// escaneando el bundle.
type Board struct {
	Columns []BoardColumn
	Count   int
}

// BoardColumn es una columna del tablero con sus tarjetas en orden
// de visualización.
type BoardColumn struct {
	Status Status
	Title  string
	Cards  []Card
}

// ToBoard agrupa un slice plano de cards en un Board, respetando el
// orden de ColumnOrder y el orden de visualización intra-columna
// (priority asc, timestamp asc, slug asc).
//
// Cards con type distinto a "backlog/card" se filtran (son OKF
// válidos pero no son tarjetas del backlog).
//
// Cards inválidas (que no pasan Validate) se filtran y se reportan
// en el segundo valor de retorno para que el caller las loguee.
func ToBoard(cards []Card) (Board, []InvalidCard) {
	var invalids []InvalidCard
	filtered := make([]Card, 0, len(cards))
	for _, c := range cards {
		if c.Type != DefaultType {
			continue
		}
		if err := c.Validate(); err != nil {
			invalids = append(invalids, InvalidCard{Path: c.Path, Reason: err.Error()})
			continue
		}
		filtered = append(filtered, c)
	}

	sortCards(filtered)

	cols := make([]BoardColumn, 0, len(ColumnOrder))
	total := 0
	for _, s := range ColumnOrder {
		col := BoardColumn{Status: s, Title: ColumnTitle(s)}
		for _, c := range filtered {
			if c.Status == s {
				col.Cards = append(col.Cards, c)
				total++
			}
		}
		cols = append(cols, col)
	}
	return Board{Columns: cols, Count: total}, invalids
}

// InvalidCard reporta una tarjeta que no pasó Validate. El FS layer
// o el caller decide si loguearla.
type InvalidCard struct {
	Path   string
	Reason string
}

// sortCards ordena por prioridad asc (P0 primero), timestamp asc
// (más vieja primero), slug asc (desempate estable). Cards sin
// timestamp se ordenan al final de su priority bucket.
func sortCards(cards []Card) {
	sort.SliceStable(cards, func(i, j int) bool {
		if cards[i].Priority != cards[j].Priority {
			// Priority es string ("P0".."P3"); el orden natural
			// del string coincide con el orden lógico porque todos
			// tienen el mismo ancho.
			return cards[i].Priority < cards[j].Priority
		}
		ti, ji := cards[i].Timestamp, cards[j].Timestamp
		if ti != ji {
			return ti < ji
		}
		return cards[i].Slug < cards[j].Slug
	})
}
