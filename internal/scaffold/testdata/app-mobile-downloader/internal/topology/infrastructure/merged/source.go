package merged

import (
	"context"
	"sort"
	"strings"

	topologyapp "gitinittest5/internal/topology/application"
)

type syncSource interface {
	ListSyncSessions(context.Context) ([]topologyapp.SyncSession, error)
}

type Source struct {
	primary  syncSource
	overlays []syncSource
}

func NewSource(primary syncSource, overlays ...syncSource) *Source {
	return &Source{primary: primary, overlays: overlays}
}

func (s *Source) ListSyncSessions(ctx context.Context) ([]topologyapp.SyncSession, error) {
	if s == nil || s.primary == nil {
		return nil, nil
	}
	base, err := s.primary.ListSyncSessions(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]topologyapp.SyncSession, len(base))
	for _, session := range base {
		if strings.TrimSpace(session.SessionID) == "" {
			continue
		}
		byID[session.SessionID] = session
	}
	for _, overlay := range s.overlays {
		if overlay == nil {
			continue
		}
		sessions, err := overlay.ListSyncSessions(ctx)
		if err != nil {
			return nil, err
		}
		for _, meta := range sessions {
			baseSession, ok := byID[meta.SessionID]
			if !ok {
				if strings.TrimSpace(meta.SessionID) == "" {
					continue
				}
				byID[meta.SessionID] = meta
				continue
			}
			if strings.TrimSpace(meta.ClientName) != "" {
				baseSession.ClientName = strings.TrimSpace(meta.ClientName)
			}
			if strings.TrimSpace(meta.Source) != "" {
				baseSession.Source = strings.TrimSpace(meta.Source)
			}
			if !meta.LastSeenAt.IsZero() {
				baseSession.LastSeenAt = meta.LastSeenAt
			}
			if !meta.StartedAt.IsZero() {
				baseSession.StartedAt = meta.StartedAt
			}
			byID[meta.SessionID] = baseSession
		}
	}
	result := make([]topologyapp.SyncSession, 0, len(byID))
	for _, session := range byID {
		result = append(result, session)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ProjectName == result[j].ProjectName {
			return result[i].SessionID < result[j].SessionID
		}
		return result[i].ProjectName < result[j].ProjectName
	})
	return result, nil
}
