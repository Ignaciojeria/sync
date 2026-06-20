package scheduler

import (
	"net/http"
	"strings"

	schedulerapp "app-mobile-downloader/internal/scheduler/application"
	schedulerpostgresql "app-mobile-downloader/internal/scheduler/infrastructure/postgresql"
	schedulerui "app-mobile-downloader/internal/scheduler/ui"
	authmiddleware "app-mobile-downloader/internal/auth/middleware"
	"app-mobile-downloader/internal/shared"
	"app-mobile-downloader/internal/shared/access"
	sharedpostgresql "app-mobile-downloader/internal/shared/infrastructure/postgresql"
	"app-mobile-downloader/internal/shared/server"
	"app-mobile-downloader/internal/ui/layout"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(jobConfigPageHandler)

func jobConfigPageHandler(s *server.Server, db *sharedpostgresql.Connection) {
	repo := schedulerpostgresql.NewJobRepository(db)

	fuego.Get(s.Server, "/scheduler/jobs", func(c fuego.ContextNoBody) (any, error) {
		claims, ok := authmiddleware.JWTClaimsFromContext(c.Context())
		if !ok {
			return nil, fuego.HTTPError{Status: http.StatusUnauthorized, Detail: "unauthorized"}
		}
		email := shared.FirstStringClaim(claims, "email")
		if !access.IsAllowedEditorEmail(email) {
			return nil, fuego.HTTPError{Status: http.StatusForbidden, Detail: "forbidden"}
		}

		configs, err := repo.FindAll()
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}

		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		page, err := layout.RenderPage(c, "Configuración de Jobs", schedulerui.JobsPage(configs))
		if err != nil {
			return nil, err
		}
		return nil, page.Render(c.Context(), c.Response())
	})

	fuego.Get(s.Server, "/scheduler/jobs/new", func(c fuego.ContextNoBody) (any, error) {
		claims, ok := authmiddleware.JWTClaimsFromContext(c.Context())
		if !ok {
			return nil, fuego.HTTPError{Status: http.StatusUnauthorized, Detail: "unauthorized"}
		}
		email := shared.FirstStringClaim(claims, "email")
		if !access.IsAllowedEditorEmail(email) {
			return nil, fuego.HTTPError{Status: http.StatusForbidden, Detail: "forbidden"}
		}
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return schedulerui.JobForm(), nil
	})

	fuego.Get(s.Server, "/scheduler/jobs/cancel", func(c fuego.ContextNoBody) (any, error) {
		claims, ok := authmiddleware.JWTClaimsFromContext(c.Context())
		if !ok {
			return nil, fuego.HTTPError{Status: http.StatusUnauthorized, Detail: "unauthorized"}
		}
		email := shared.FirstStringClaim(claims, "email")
		if !access.IsAllowedEditorEmail(email) {
			return nil, fuego.HTTPError{Status: http.StatusForbidden, Detail: "forbidden"}
		}
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return schedulerui.EmptyForm(), nil
	})

	fuego.Post(s.Server, "/scheduler/jobs", func(c fuego.ContextNoBody) (any, error) {
		claims, ok := authmiddleware.JWTClaimsFromContext(c.Context())
		if !ok {
			return nil, fuego.HTTPError{Status: http.StatusUnauthorized, Detail: "unauthorized"}
		}
		email := shared.FirstStringClaim(claims, "email")
		if !access.IsAllowedEditorEmail(email) {
			return nil, fuego.HTTPError{Status: http.StatusForbidden, Detail: "forbidden"}
		}

		if err := c.Request().ParseForm(); err != nil {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
		}

		job := schedulerapp.JobConfig{
			Name:        strings.TrimSpace(c.Request().FormValue("name")),
			Description: strings.TrimSpace(c.Request().FormValue("description")),
			Schedule:    strings.TrimSpace(c.Request().FormValue("schedule")),
			Endpoint:    strings.TrimSpace(c.Request().FormValue("endpoint")),
			Enabled:     false,
		}
		if job.Name == "" || job.Schedule == "" || job.Endpoint == "" {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: "name, schedule and endpoint are required"}
		}

		id, err := repo.Create(job)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}

		created, err := repo.FindByID(id)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}

		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return schedulerui.JobRow(created), nil
	})

	fuego.Delete(s.Server, "/scheduler/jobs/{id}", func(c fuego.ContextNoBody) (any, error) {
		claims, ok := authmiddleware.JWTClaimsFromContext(c.Context())
		if !ok {
			return nil, fuego.HTTPError{Status: http.StatusUnauthorized, Detail: "unauthorized"}
		}
		email := shared.FirstStringClaim(claims, "email")
		if !access.IsAllowedEditorEmail(email) {
			return nil, fuego.HTTPError{Status: http.StatusForbidden, Detail: "forbidden"}
		}

		id := c.PathParam("id")
		if id == "" {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: "missing job id"}
		}

		if err := repo.Delete(id); err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}

		return nil, nil
	})

	fuego.Post(s.Server, "/scheduler/jobs/{id}/toggle", func(c fuego.ContextNoBody) (any, error) {
		claims, ok := authmiddleware.JWTClaimsFromContext(c.Context())
		if !ok {
			return nil, fuego.HTTPError{Status: http.StatusUnauthorized, Detail: "unauthorized"}
		}
		email := shared.FirstStringClaim(claims, "email")
		if !access.IsAllowedEditorEmail(email) {
			return nil, fuego.HTTPError{Status: http.StatusForbidden, Detail: "forbidden"}
		}

		id := c.PathParam("id")
		if id == "" {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: "missing job id"}
		}

		job, err := repo.FindByID(id)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusNotFound, Detail: "job not found"}
		}

		if err := repo.UpdateEnabled(id, !job.Enabled); err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}

		updated, err := repo.FindByID(id)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}

		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return schedulerui.JobRow(updated), nil
	})
}

var _ = ioc.Register(jobConfigAPIHandler)

func jobConfigAPIHandler(s *server.Server, db *sharedpostgresql.Connection) {
	repo := schedulerpostgresql.NewJobRepository(db)

	fuego.Get(s.Server, "/api/internal/jobs", func(c fuego.ContextNoBody) (any, error) {
		configs, err := repo.FindAll()
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}
		return configs, nil
	})

	fuego.Post(s.Server, "/api/internal/jobs/{id}/run", func(c fuego.ContextNoBody) (any, error) {
		id := c.PathParam("id")
		if id == "" {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: "missing job id"}
		}

		job, err := repo.FindByID(id)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusNotFound, Detail: "job not found"}
		}
		if !job.Enabled {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: "job is disabled"}
		}

		return map[string]string{"status": "triggered", "job": job.Name}, nil
	})
}
