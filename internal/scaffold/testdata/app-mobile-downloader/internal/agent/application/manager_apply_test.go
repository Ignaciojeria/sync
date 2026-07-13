package application

import (
	"context"
	"errors"
	"testing"
)

func TestManagerApplyPreview_UsesInjectedApplier(t *testing.T) {
	store := newStubStore()
	manager := NewManager(store, &factoryRunner{}).WithSessionApplier(func(_ context.Context, session Session) (ApplyResult, error) {
		return ApplyResult{SourcePath: session.SourcePath, PreviewPath: session.WorkspacePath}, nil
	})
	ctx := t.Context()
	session, err := manager.Create(ctx, CreateSessionInput{Title: "apply", CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	stored, _ := store.Get(ctx, session.ID)
	stored.SourcePath = t.TempDir()
	stored.WorkspacePath = t.TempDir()
	_ = store.Update(ctx, stored)
	result, err := manager.ApplyPreview(ctx, session.ID)
	if err != nil {
		t.Fatalf("ApplyPreview: %v", err)
	}
	if result.SourcePath == "" || result.PreviewPath == "" {
		t.Fatal("apply result vacío")
	}
}

func TestManagerApplyPreview_RejectsNonApplicableSession(t *testing.T) {
	store := newStubStore()
	manager := NewManager(store, &factoryRunner{})
	ctx := t.Context()
	session, _ := manager.Create(ctx, CreateSessionInput{Title: "apply", CWD: t.TempDir()})
	_, err := manager.ApplyPreview(ctx, session.ID)
	if !errors.Is(err, ErrPreviewNotApplicable) {
		t.Fatalf("err = %v, want ErrPreviewNotApplicable", err)
	}
}
