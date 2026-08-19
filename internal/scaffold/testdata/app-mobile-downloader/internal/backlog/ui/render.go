package ui

import (
	"context"
	"io"

	"gitinittest5/internal/backlog/application"
)

// RenderBoard escribe el tablero completo. Pensado para la página
// principal; los handlers que actualizan una sola pieza usan los
// componentes directamente.
func RenderBoard(w io.Writer, ctx context.Context, b application.Board, previewPrefix string) error {
	return Board(ToBoardView(b), previewPrefix).Render(ctx, w)
}

// RenderCardForReplace escribe una tarjeta con hx-swap-oob que la
// reemplaza en su lugar actual. Usado por los handlers de Priority y
// Update.
func RenderCardForReplace(w io.Writer, ctx context.Context, c application.Card, previewPrefix string) error {
	return Card(ToCardView(c), previewPrefix, SwapReplace).Render(ctx, w)
}

// RenderCardForMove escribe la respuesta de un Move: borra la tarjeta
// de su columna origen y la inserta al final de la columna destino.
func RenderCardForMove(w io.Writer, ctx context.Context, c application.Card, previewPrefix string) error {
	if err := DeleteOutOfBand(c.Slug).Render(ctx, w); err != nil {
		return err
	}
	return Card(ToCardView(c), previewPrefix, SwapInsert).Render(ctx, w)
}

// RenderDeleteOutOfBand escribe un <template hx-swap-oob="delete:..."
// que, al recibirse como respuesta HTMX, hace desaparecer la tarjeta
// con ese slug. Usado por el handler de Delete.
func RenderDeleteOutOfBand(w io.Writer, ctx context.Context, slug string) error {
	return DeleteOutOfBand(slug).Render(ctx, w)
}

// RenderDetailFragment escribe el detalle de una tarjeta tal como se
// inyecta dentro del <dialog> del board. NO incluye el shell del
// modal: ese vive en CardDetailModalShell y se monta una sola vez en
// el board. previewPrefix se propaga para que las acciones dentro
// del detalle (mover, priorizar, eliminar) generen URLs correctas.
func RenderDetailFragment(w io.Writer, ctx context.Context, c application.Card, previewPrefix string) error {
	return Detail(ToCardView(c), previewPrefix).Render(ctx, w)
}

// RenderColumn escribe una columna completa (header + todas sus
// tarjetas). Usado como OOB-swap al recargar el tablero.
func RenderColumn(w io.Writer, ctx context.Context, col application.BoardColumn, previewPrefix string) error {
	return Column(ColumnView{
		Status: col.Status,
		Title:  col.Title,
		Cards:  cardsToViews(col.Cards),
	}, previewPrefix).Render(ctx, w)
}

func cardsToViews(cards []application.Card) []CardView {
	out := make([]CardView, 0, len(cards))
	for _, c := range cards {
		out = append(out, ToCardView(c))
	}
	return out
}
