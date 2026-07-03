package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	agentapp "app-mobile-downloader/pkg/agent/application"
	"app-mobile-downloader/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func eventsHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	fuego.Get(s.Server, "/agent/sessions/{id}/events", streamEvents(manager), fuego.OptionMiddleware(requireEditor))
}

func streamEvents(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		id, err := pathSessionID(c)
		if err != nil {
			return nil, err
		}
		ch, cancel, err := manager.Subscribe(c.Context(), id)
		if err != nil {
			return nil, mapSessionError(err)
		}
		defer cancel()

		flusher, ok := c.Response().(http.Flusher)
		if !ok {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: "streaming not supported"}
		}

		w := c.Response()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		if err := writeSSE(w, "status", map[string]any{"status": "connected", "sessionId": id}); err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}
		flusher.Flush()

		// Keepalive de 10s para ganarle al proxy inverso de exe.dev, que
		// corta conexiones idle en torno a 30 s (visible en el log como
		// duration_ms ≈ 30 000). Con un keepalive más corto la conexión
		// siempre lleva tráfico fresco y el navegador no la recicla.
		keepalive := time.NewTicker(10 * time.Second)
		defer keepalive.Stop()

		ctx := c.Context()
		for {
			select {
			case <-ctx.Done():
				return nil, nil
			case event, ok := <-ch:
				if !ok {
					return nil, nil
				}
				if err := writeSSE(w, "pi", event); err != nil {
					return nil, nil
				}
				flusher.Flush()
			case <-keepalive.C:
				if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
					return nil, nil
				}
				flusher.Flush()
			}
		}
	}
}

func writeSSE(w http.ResponseWriter, eventName string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, payload)
	return err
}

var _ = agentapp.Event{}
