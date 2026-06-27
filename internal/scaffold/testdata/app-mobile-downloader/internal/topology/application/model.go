package application

import "time"

const (
	StatusRunning  = "running"
	StatusSyncing  = "syncing"
	StatusDegraded = "degraded"
	StatusOffline  = "offline"
)

type Workspace struct {
	Name      string
	Status    string
	Summary   string
	UpdatedAt time.Time
}

type ServiceNode struct {
	Name      string
	Kind      string
	Status    string
	Summary   string
	UpdatedAt time.Time
}

type SyncSession struct {
	SessionID   string
	ProjectName string
	ClientName  string
	Status      string
	Source      string
	StartedAt   time.Time
	LastSeenAt  time.Time
}

type Snapshot struct {
	Workspace    Workspace
	Services     []ServiceNode
	SyncSessions []SyncSession
	GeneratedAt  time.Time
}
