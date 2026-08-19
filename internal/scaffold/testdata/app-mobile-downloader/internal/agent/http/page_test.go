package agent

import (
	"context"
	"testing"

	agentapp "lastmile-agents/internal/agent/application"
)

type pageServiceStub struct {
	sessions map[string]agentapp.Session
	order    []agentapp.Session
	err      error
}

func (s pageServiceStub) List(context.Context) ([]agentapp.Session, error) {
	if s.order != nil {
		return s.order, s.err
	}
	out := make([]agentapp.Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		out = append(out, session)
	}
	return out, s.err
}
func (s pageServiceStub) Create(context.Context, agentapp.CreateSessionInput) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s pageServiceStub) Get(_ context.Context, id string) (agentapp.Session, error) {
	if session, ok := s.sessions[id]; ok {
		return session, nil
	}
	return agentapp.Session{}, agentapp.ErrSessionNotFound
}
func (s pageServiceStub) Ensure(context.Context, string) error         { return nil }
func (s pageServiceStub) Prompt(context.Context, string, string) error { return nil }
func (s pageServiceStub) PromptRequest(context.Context, string, agentapp.PromptInput) error {
	return nil
}
func (s pageServiceStub) Steer(context.Context, string, string) error { return nil }
func (s pageServiceStub) Abort(context.Context, string) error         { return nil }
func (s pageServiceStub) Regenerate(context.Context, string) error   { return nil }
func (s pageServiceStub) Subscribe(context.Context, string) (<-chan agentapp.Event, func(), error) {
	return nil, func() {}, nil
}
func (s pageServiceStub) RegisterPreview(context.Context, string, agentapp.RegisterPreviewInput) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s pageServiceStub) ClearPreview(context.Context, string) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s pageServiceStub) ApplyPreview(context.Context, string) (agentapp.ApplyResult, error) {
	return agentapp.ApplyResult{}, nil
}
func (s pageServiceStub) MergePreview(context.Context, string) (agentapp.MergeResult, error) {
	return agentapp.MergeResult{}, nil
}
func (s pageServiceStub) Delete(context.Context, string) error { return nil }
func (s pageServiceStub) Close() error                         { return nil }

func TestResolveAgentEntryRedirect_UsesLatestSession(t *testing.T) {
	got := resolveAgentEntryRedirect(t.Context(), pageServiceStub{sessions: map[string]agentapp.Session{"s1": {ID: "s1"}}, order: []agentapp.Session{{ID: "s1"}}}, "", "", "/agent")
	if want := "/agent?session=s1"; got != want {
		t.Fatalf("redirect = %q, want %q", got, want)
	}
}

func TestResolveAgentEntryRedirect_FallsBackToSessionsPage(t *testing.T) {
	got := resolveAgentEntryRedirect(t.Context(), pageServiceStub{}, "", "", "/agent")
	if want := "/agent/home"; got != want {
		t.Fatalf("redirect = %q, want %q", got, want)
	}
}

func TestResolveAgentEntryRedirect_RespectsMountedPrefix(t *testing.T) {
	got := resolveAgentEntryRedirect(t.Context(), pageServiceStub{sessions: map[string]agentapp.Session{"s1": {ID: "s1"}}, order: []agentapp.Session{{ID: "s1"}}}, "/agent/sessions/p1/preview/", "", "/agent")
	if want := "/agent/sessions/p1/preview/agent?session=s1"; got != want {
		t.Fatalf("redirect = %q, want %q", got, want)
	}
}

func TestResolveAgentEntryRedirect_SkipsWhenSessionAlreadySelected(t *testing.T) {
	got := resolveAgentEntryRedirect(t.Context(), pageServiceStub{sessions: map[string]agentapp.Session{"s9": {ID: "s9"}}, order: []agentapp.Session{{ID: "s9"}}}, "", "s9", "/agent")
	if got != "" {
		t.Fatalf("redirect = %q, want empty", got)
	}
}

func TestResolveAgentEntryRedirect_IgnoresPreviewOwnerSessionID(t *testing.T) {
	got := resolveAgentEntryRedirect(t.Context(), pageServiceStub{sessions: map[string]agentapp.Session{"p1": {ID: "p1"}}, order: []agentapp.Session{{ID: "p1"}}}, "/agent/sessions/p1/preview/", "p1", "/agent")
	if want := "/agent/sessions/p1/preview/agent/home"; got != want {
		t.Fatalf("redirect = %q, want %q", got, want)
	}
}

func TestResolveAgentEntryRedirect_SkipsPreviewOwnerWhenPickingDefaultSession(t *testing.T) {
	got := resolveAgentEntryRedirect(t.Context(), pageServiceStub{sessions: map[string]agentapp.Session{"p1": {ID: "p1"}, "s2": {ID: "s2"}}, order: []agentapp.Session{{ID: "p1"}, {ID: "s2"}}}, "/agent/sessions/p1/preview/", "", "/agent")
	if want := "/agent/sessions/p1/preview/agent?session=s2"; got != want {
		t.Fatalf("redirect = %q, want %q", got, want)
	}
}

func TestResolveAgentEntryRedirect_RespectsV2BasePath(t *testing.T) {
	// ponytail: V2 usa el mismo helper pero con basePath="/agent-v2".
	// El redirect tiene que apuntar a /agent-v2/home y no a /agent/home.
	got := resolveAgentEntryRedirect(t.Context(), pageServiceStub{}, "", "", "/agent-v2")
	if want := "/agent-v2/home"; got != want {
		t.Fatalf("redirect = %q, want %q", got, want)
	}
}

func TestResolveAgentEntryRedirect_NormalizesBasePath(t *testing.T) {
	// ponytail: el helper debe tolerar basePath con o sin slash
	// inicial y con o sin slash final. Si llega "agent-v2/" debe
	// normalizarse a "/agent-v2".
	got := resolveAgentEntryRedirect(t.Context(), pageServiceStub{}, "", "", "agent-v2/")
	if want := "/agent-v2/home"; got != want {
		t.Fatalf("redirect = %q, want %q", got, want)
	}
}
