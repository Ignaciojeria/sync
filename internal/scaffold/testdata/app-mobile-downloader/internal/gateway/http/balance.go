package http

import (
	gatewayapp "fixtests1/internal/gateway/application"
	gatewayui "fixtests1/internal/gateway/ui"
	"fixtests1/internal/shared/server"
	"fmt"
	"github.com/a-h/templ"
	"net/http"
	"strings"
)

func Register(s *server.Server, balanceSvc *gatewayapp.BalanceService, sessionCostSvc *gatewayapp.SessionCostService) {
	server.Get(s, "/gateway/balance", balanceHandler(balanceSvc))
	server.Get(s, "/gateway/session-cost", sessionCostHandler(sessionCostSvc))
}

func balanceHandler(svc *gatewayapp.BalanceService) func(server.ContextNoBody) (any, error) {
	return func(c server.ContextNoBody) (any, error) {
		bal, err := svc.Fetch(c.Context())
		variant := c.Request().URL.Query().Get("variant")
		// ?format=json: lo consume la barra de presupuesto del chat
		// (internal/agent/ui/page.templ) sin re-renderizar HTML. El resto
		// de la UI sigue usando el badge HTMX como hasta ahora.
		if c.Request().URL.Query().Get("format") == "json" {
			if err != nil {
				return nil, server.HTTPError{Status: http.StatusBadGateway, Detail: err.Error()}
			}
			return bal, nil
		}
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		// ponytail: error → "—" en vez de 500. La UI sigue viva aunque
		// el gateway esté caído. Upgrade path: retry + toast en el cliente.
		amount := "—"
		if err == nil {
			amount = fmt.Sprintf("$%.2f", bal.BalanceUSD)
		}
		var component templ.Component
		if variant == "compact" {
			component = gatewayui.CompactBadge(amount)
		} else {
			component = gatewayui.Badge(amount)
		}
		return nil, component.Render(c.Context(), c.Response())
	}
}

func sessionCostHandler(svc *gatewayapp.SessionCostService) func(server.ContextNoBody) (any, error) {
	return func(c server.ContextNoBody) (any, error) {
		sessionID := strings.TrimSpace(c.Request().URL.Query().Get("session_id"))
		if sessionID == "" {
			return nil, server.HTTPError{Status: http.StatusBadRequest, Detail: "session_id is required"}
		}
		cost, err := svc.Fetch(c.Context(), sessionID)
		if err != nil {
			return nil, server.HTTPError{Status: http.StatusBadGateway, Detail: err.Error()}
		}
		return cost, nil
	}
}
