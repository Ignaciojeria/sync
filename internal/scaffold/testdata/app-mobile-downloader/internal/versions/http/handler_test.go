package versions

import (
	"context"
	"errors"
	"testing"

	versionsapp "gitinittest5/internal/versions/application"
	versionsui "gitinittest5/internal/versions/ui"
)

// stubReader implementa versionsapp.Reader sin tocar git. Lo usamos
// para verificar contratos en aislamiento y para posibles tests E2E
// del handler sin depender del sistema de archivos.
type stubReader struct {
	list []versionsapp.Version
	err  error
}

func (s stubReader) List(_ context.Context, _ int) ([]versionsapp.Version, error) {
	return s.list, s.err
}
func (s stubReader) Get(_ context.Context, sha string) (versionsapp.Version, error) {
	if s.err != nil {
		return versionsapp.Version{}, s.err
	}
	for _, v := range s.list {
		if v.SHA == sha || v.ShortSHA == sha {
			return v, nil
		}
	}
	return versionsapp.Version{}, errors.New("not found")
}
func (s stubReader) Diff(_ context.Context, _ string) ([]versionsapp.VersionFile, error) {
	return nil, nil
}

// TestStubReaderImplementsContract garantiza que el stub satisface
// la interfaz. Si la interfaz agrega métodos, este test rompe antes
// de que el handler deje de compilar.
func TestStubReaderImplementsContract(t *testing.T) {
	var _ versionsapp.Reader = stubReader{}
}

// TestListStatePropagation verifica que los datos del reader llegan
// tal cual al state que consume el template. La validación del HTML
// rendered se hace con templ en aislamiento en otro test.
func TestListStatePropagation(t *testing.T) {
	r := stubReader{list: []versionsapp.Version{
		{ShortSHA: "abc1234", Message: "Merge branch 'x'"},
	}}
	items, err := r.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	state := versionsui.ListState{Versions: items}
	if len(state.Versions) != 1 || state.Versions[0].ShortSHA != "abc1234" {
		t.Fatalf("state mal armado: %+v", state)
	}
	if state.Error != "" {
		t.Fatalf("Error debería estar vacío: %q", state.Error)
	}
}

// TestGetReturnsErrorWhenMissing valida que el reader propaga el
// error correctamente para que el handler pueda responder 404.
func TestGetReturnsErrorWhenMissing(t *testing.T) {
	r := stubReader{list: nil}
	_, err := r.Get(context.Background(), "nope")
	if err == nil {
		t.Fatalf("expected error for missing sha")
	}
}