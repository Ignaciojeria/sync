package application

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

// recorderRenderer es un FragmentRenderer que sólo recuerda qué items
// le pidieron renderizar. Útil para verificar que el lookup por
// sesión elige el renderer correcto y que cada renderer recibe
// únicamente los items de su sesión.
type recorderRenderer struct {
	mu    sync.Mutex
	tag   string
	items []ConversationItem
}

func (r *recorderRenderer) RenderFragment(item ConversationItem) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, item)
	return `<div data-renderer="` + r.tag + `">` + item.Text + `</div>`, nil
}

// RenderToolResultPartial es un stub del test que NO se usa
// (los tests del registry no ejercen streaming de tool). Lo
// implementamos para satisfacer la interfaz FragmentRenderer.
func (r *recorderRenderer) RenderToolResultPartial(toolCallID, toolName, text string) (string, error) {
	return "", nil
}

func (r *recorderRenderer) Items() []ConversationItem {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ConversationItem, len(r.items))
	copy(out, r.items)
	return out
}

func resetRegistry(t *testing.T) {
	t.Helper()
	globalRegistry.mu.Lock()
	globalRegistry.fallback = nil
	globalRegistry.perScope = map[string]FragmentRenderer{}
	globalRegistry.mu.Unlock()
}

func TestRegistry_FallbackUsedWhenNoScopeRegistered(t *testing.T) {
	defer resetRegistry(t)
	fb := &recorderRenderer{tag: "fb"}
	SetFragmentRenderer(fb)

	got := rendererFor("sesion-inexistente")
	if got != FragmentRenderer(fb) {
		t.Fatalf("rendererFor debería devolver el fallback cuando no hay scope, got %T", got)
	}
}

func TestRegistry_PerScopeWinsOverFallback(t *testing.T) {
	defer resetRegistry(t)
	fb := &recorderRenderer{tag: "fb"}
	scoped := &recorderRenderer{tag: "v2"}
	SetFragmentRenderer(fb)
	SetSessionRenderer("sesion-v2", scoped)

	if got := rendererFor("sesion-v2"); got != FragmentRenderer(scoped) {
		t.Fatalf("rendererFor debería devolver el scoped renderer para sesion-v2, got %T", got)
	}
	if got := rendererFor("otra-sesion"); got != FragmentRenderer(fb) {
		t.Fatalf("rendererFor debería devolver el fallback para otras sesiones, got %T", got)
	}
}

func TestRegistry_ClearSessionRenderer(t *testing.T) {
	defer resetRegistry(t)
	fb := &recorderRenderer{tag: "fb"}
	scoped := &recorderRenderer{tag: "v2"}
	SetFragmentRenderer(fb)
	SetSessionRenderer("sesion-v2", scoped)

	ClearSessionRenderer("sesion-v2")
	if got := rendererFor("sesion-v2"); got != FragmentRenderer(fb) {
		t.Fatalf("tras Clear, rendererFor debería devolver el fallback, got %T", got)
	}
}

func TestRegistry_NilInputsAreNoop(t *testing.T) {
	defer resetRegistry(t)

	// nil renderer no rompe nada.
	SetSessionRenderer("", nil)
	SetSessionRenderer("sesion", nil)

	if got := rendererFor("sesion"); got != nil {
		t.Fatalf("sin nada registrado, rendererFor debería devolver nil, got %T", got)
	}

	// Clear sobre id vacío no panic.
	ClearSessionRenderer("")
}

func TestStreamOrSkip_PerScopeRendererReceivesItem(t *testing.T) {
	// ponytail: el cambio de registry no debe romper el contrato
	// del streamOrSkip — un renderer per-session debe recibir los
	// items de streaming de su sesión, no del fallback.
	defer resetRegistry(t)
	fb := &recorderRenderer{tag: "fb"}
	v2 := &recorderRenderer{tag: "v2"}
	SetFragmentRenderer(fb)
	SetSessionRenderer("sesion-v2", v2)

	// Pre-poblamos el draft para esa sesión como si message_start
	// hubiera llegado antes.
	MaterializeEvent("sesion-v2", 7, Event{Type: "message_start"})

	event := Event{
		Type:    "message_update",
		Payload: []byte(`{"assistantMessageEvent":{"type":"text_delta","delta":"hola v2"}}`),
	}
	fragment, ok := RenderFragment("sesion-v2", 8, event)
	if !ok {
		t.Fatalf("RenderFragment debería emitir fragment")
	}
	if !strings.Contains(fragment.HTML, `data-renderer="v2"`) {
		t.Fatalf("el HTML debería provenir del renderer per-session, got %s", fragment.HTML)
	}
	if !strings.Contains(fragment.HTML, "hola v2") {
		t.Fatalf("HTML no contiene el delta: %s", fragment.HTML)
	}
	if len(fb.Items()) != 0 {
		t.Fatalf("el fallback no debería haber sido llamado, got %d items", len(fb.Items()))
	}
	if len(v2.Items()) != 1 {
		t.Fatalf("v2 renderer debería haber recibido 1 item, got %d", len(v2.Items()))
	}
}

func TestEmitFragment_PerScopeRenderer(t *testing.T) {
	defer resetRegistry(t)
	fb := &recorderRenderer{tag: "fb"}
	v2 := &recorderRenderer{tag: "v2"}
	SetFragmentRenderer(fb)
	SetSessionRenderer("sesion-v2", v2)

	// Materializamos un item assistant completo en el transcript.
	MaterializeEvent("sesion-v2", 20, Event{Type: "message_start"})
	MaterializeEvent("sesion-v2", 21, Event{
		Type:    "message_update",
		Payload: []byte(`{"assistantMessageEvent":{"type":"text_delta","delta":"respuesta final"}}`),
	})
	MaterializeEvent("sesion-v2", 22, Event{Type: "message_end"})

	fragment, ok := EmitFragment("sesion-v2", 22)
	if !ok {
		t.Fatalf("EmitFragment debería emitir fragment para sesion-v2")
	}
	if !strings.Contains(fragment.HTML, `data-renderer="v2"`) {
		t.Fatalf("EmitFragment debería usar el renderer per-session, got %s", fragment.HTML)
	}
	if len(fb.Items()) != 0 {
		t.Fatalf("fallback no debería haber sido llamado")
	}
}

type errRenderer struct{}

func (errRenderer) RenderFragment(item ConversationItem) (string, error) {
	return "", errors.New("renderer boom")
}

// RenderToolResultPartial es un stub que devuelve el mismo
// error que RenderFragment. NO se usa en este test (cubre el
// path de message_update → streamOrSkip, no de tool streaming)
// pero la interfaz lo requiere.
func (errRenderer) RenderToolResultPartial(toolCallID, toolName, text string) (string, error) {
	return "", errors.New("renderer boom")
}

func TestStreamOrSkip_RendererErrorSkipsFragment(t *testing.T) {
	defer resetRegistry(t)
	SetSessionRenderer("s", errRenderer{})
	MaterializeEvent("s", 30, Event{Type: "message_start"})

	event := Event{
		Type:    "message_update",
		Payload: []byte(`{"assistantMessageEvent":{"type":"text_delta","delta":"x"}}`),
	}
	if _, ok := RenderFragment("s", 31, event); ok {
		t.Fatalf("RenderFragment debería devolver false cuando el renderer falla")
	}
}
