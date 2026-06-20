CREATE TABLE IF NOT EXISTS job_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT false,
    schedule TEXT NOT NULL DEFAULT '*/5 * * * *',
    endpoint TEXT NOT NULL,
    last_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS job_run_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_config_id UUID NOT NULL REFERENCES job_configs(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    output TEXT,
    error_message TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    acquired_lock BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX IF NOT EXISTS idx_job_run_logs_job_config_id ON job_run_logs(job_config_id);
CREATE INDEX IF NOT EXISTS idx_job_run_logs_started_at ON job_run_logs(started_at);

INSERT INTO job_configs (name, description, enabled, schedule, endpoint)
VALUES ('anomaly_detector', 'Detecta anomalías en subórdenes cada 5 minutos', false, '*/5 * * * *', '/api/internal/anomaly/detect')
ON CONFLICT (name) DO NOTHING;
