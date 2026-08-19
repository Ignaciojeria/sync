package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestManagerMergePreview_PersistsMergedMetadata(t *testing.T) {
	store := newStubStore()
	manager := NewManager(store, &factoryRunner{})
	manager.now = func() time.Time { return time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC) }
	manager = manager.WithSessionMerger(func(_ context.Context, session Session) (MergeResult, error) {
		return MergeResult{BaseBranch: session.BaseBranch, PreviewBranch: session.Branch, Commit: "abc123"}, nil
	})
	ctx := t.Context()

	session, err := manager.Create(ctx, CreateSessionInput{Title: "merge", CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	stored, err := store.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	stored.Branch = "agent/" + session.ID
	stored.BaseBranch = "main"
	if err := store.Update(ctx, stored); err != nil {
		t.Fatalf("store.Update: %v", err)
	}

	result, err := manager.MergePreview(ctx, session.ID)
	if err != nil {
		t.Fatalf("MergePreview: %v", err)
	}
	if got, want := result.Commit, "abc123"; got != want {
		t.Fatalf("Commit = %q, want %q", got, want)
	}
	merged, err := store.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("store.Get merged: %v", err)
	}
	if merged.MergedAt == nil {
		t.Fatal("MergedAt nil")
	}
	if got, want := merged.MergedCommit, "abc123"; got != want {
		t.Fatalf("MergedCommit = %q, want %q", got, want)
	}
}

func TestManagerMergePreview_NoChangesDoesNotPersistMergedMetadata(t *testing.T) {
	// ponytail: cuando mergeSession devuelve NoChanges=true, el
	// Manager trata el resultado como éxito sin integración real:
	// NO setea MergedAt ni MergedCommit ni actualiza branches. Si
	// no, el bar mostraría "Applied" erróneamente y la sesión
	// quedaría inutilizable (un segundo click devolvería
	// ErrPreviewAlreadyMerged sin haber integrado nada).
	store := newStubStore()
	manager := NewManager(store, &factoryRunner{})
	manager = manager.WithSessionMerger(func(_ context.Context, session Session) (MergeResult, error) {
		return MergeResult{
			BaseBranch:    session.BaseBranch,
			PreviewBranch: session.Branch,
			NoChanges:     true,
		}, nil
	})
	ctx := t.Context()

	session, err := manager.Create(ctx, CreateSessionInput{Title: "merge-nochanges", CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	stored, err := store.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	stored.Branch = "agent/" + session.ID
	stored.BaseBranch = "main"
	if err := store.Update(ctx, stored); err != nil {
		t.Fatalf("store.Update: %v", err)
	}

	result, err := manager.MergePreview(ctx, session.ID)
	if err != nil {
		t.Fatalf("MergePreview: %v", err)
	}
	if !result.NoChanges {
		t.Fatalf("result.NoChanges = false, want true")
	}
	reloaded, err := store.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("store.Get after: %v", err)
	}
	if reloaded.MergedAt != nil {
		t.Fatalf("MergedAt = %v, want nil para noChanges", reloaded.MergedAt)
	}
	if strings.TrimSpace(reloaded.MergedCommit) != "" {
		t.Fatalf("MergedCommit = %q, want empty para noChanges", reloaded.MergedCommit)
	}
	// Un segundo merge con noChanges debe seguir funcionando (la
	// sesión no quedó inutilizable).
	if _, err := manager.MergePreview(ctx, session.ID); err != nil {
		t.Fatalf("second MergePreview (noChanges idempotent): %v", err)
	}
}

func TestManagerMergePreview_MapsBlockedAndConflictErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{name: "blocked", err: errors.New("worktree: base repo has uncommitted changes"), want: ErrPreviewMergeBlocked},
		{name: "conflict", err: errors.New("worktree: merge conflict happened"), want: ErrPreviewMergeConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newStubStore()
			manager := NewManager(store, &factoryRunner{}).WithSessionMerger(func(context.Context, Session) (MergeResult, error) {
				return MergeResult{}, tc.err
			})
			ctx := t.Context()
			session, _ := manager.Create(ctx, CreateSessionInput{Title: "merge", CWD: t.TempDir()})
			stored, _ := store.Get(ctx, session.ID)
			stored.Branch = "agent/" + session.ID
			stored.BaseBranch = "main"
			_ = store.Update(ctx, stored)
			_, err := manager.MergePreview(ctx, session.ID)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
