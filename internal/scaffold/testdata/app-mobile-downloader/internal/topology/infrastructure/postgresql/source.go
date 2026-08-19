package postgresql

import (
	"context"
	"fmt"
	"time"

	topologyapp "gitinittest5/internal/topology/application"
)

type pinger interface {
	PingContext(context.Context) error
}

type Source struct {
	db  pinger
	now func() time.Time
}

func NewSource(db pinger) *Source {
	return &Source{db: db, now: time.Now}
}

func (s *Source) ListServices(ctx context.Context) ([]topologyapp.ServiceNode, error) {
	now := s.now()
	node := topologyapp.ServiceNode{
		Name:      "PostgreSQL",
		Kind:      "database",
		Status:    topologyapp.StatusOffline,
		Summary:   "No database connection configured",
		UpdatedAt: now,
	}
	if s.db == nil {
		return []topologyapp.ServiceNode{node}, nil
	}
	if err := s.db.PingContext(ctx); err != nil {
		node.Status = topologyapp.StatusDegraded
		node.Summary = fmt.Sprintf("Database unreachable: %v", err)
		return []topologyapp.ServiceNode{node}, nil
	}
	node.Status = topologyapp.StatusRunning
	node.Summary = "Database connection healthy"
	return []topologyapp.ServiceNode{node}, nil
}
