package scheduler

import (
	"net/http"
	"strings"

	schedulerapp "testboi1/internal/scheduler/application"
	schedulerpostgresql "testboi1/internal/scheduler/infrastructure/postgresql"
	schedulerui "testboi1/internal/scheduler/ui"
	authmiddleware "testboi1/internal/auth/middleware"
	sharedpostgresql "testboi1/internal/shared/infrastructure/postgresql"
	"testboi1/internal/shared/server"
	"testboi1/internal/ui/layout"

	"github.com/go-fuego/fuego"
)

func jobConfigPageHandler(s *server.Server, db *sharedpostgresql.Connection) {
	repo := schedulerpostgresql.NewJobRepository(db)
	requireEditor := fuego.OptionMiddleware(authmiddleware.RequireEditor())

	fuego.Get(s.Server, "/scheduler/jobs", listJobsPage(repo), requireEditor)
	fuego.Get(s.Server, "/scheduler/jobs/new", newJobForm(), requireEditor)
	fuego.Get(s.Server, "/scheduler/jobs/cancel", cancelJobForm(), requireEditor)
	fuego.Post(s.Server, "/scheduler/jobs", createJob(repo), requireEditor)
	fuego.Delete(s.Server, "/scheduler/jobs/{id}", deleteJob(repo), requireEditor)
	fuego.Post(s.Server, "/scheduler/jobs/{id}/toggle", toggleJob(repo), requireEditor)
}

func listJobsPage(repo *schedulerpostgresql.JobRepository) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		configs, err := repo.FindAll()
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}
		nav := layout.FromRequest(c.Request())
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		page, err := layout.RenderPage(c, "Configuración de Jobs", schedulerui.JobsPage(configs, nav.PreviewPrefix))
		if err != nil {
			return nil, err
		}
		return nil, page.Render(c.Context(), c.Response())
	}
}

func newJobForm() func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		nav := layout.FromRequest(c.Request())
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return schedulerui.JobForm(nav.PreviewPrefix), nil
	}
}

func cancelJobForm() func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return schedulerui.EmptyForm(), nil
	}
}

func createJob(repo *schedulerpostgresql.JobRepository) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
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

		nav := layout.FromRequest(c.Request())
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return schedulerui.JobRow(created, nav.PreviewPrefix), nil
	}
}

func deleteJob(repo *schedulerpostgresql.JobRepository) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		id := c.PathParam("id")
		if id == "" {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: "missing job id"}
		}

		if err := repo.Delete(id); err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}
		return nil, nil
	}
}

func toggleJob(repo *schedulerpostgresql.JobRepository) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
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

		nav := layout.FromRequest(c.Request())
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return schedulerui.JobRow(updated, nav.PreviewPrefix), nil
	}
}

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
