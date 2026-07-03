package application

import (
	"encoding/json"
	"time"
)

// Event es la unidad mínima publicada hacia SSE y otros subscribers.
type Event struct {
	SessionID string          `json:"sessionId"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"createdAt"`
}
