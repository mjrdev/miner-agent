-- +goose Up
CREATE TABLE runs (
    id                  uuid          PRIMARY KEY,
    task_id             uuid          NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    status              text          NOT NULL
        CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    agent               text          NOT NULL,
    model               text          NOT NULL,
    effort              text,
    runner              text          NOT NULL,
    repo_url            text          NOT NULL,
    base_branch         text          NOT NULL,
    branch              text          NOT NULL,
    auth_method         text,
    effective_model     text,
    cost_usd            numeric(12, 6),
    num_turns           integer,
    exit_code           integer,
    error               text,
    diff                text,
    cancel_requested_at timestamptz,
    created_at          timestamptz   NOT NULL,
    started_at          timestamptz,
    finished_at         timestamptz
);

-- A task has at most one active run.
CREATE UNIQUE INDEX runs_active_task_idx ON runs (task_id) WHERE status IN ('queued', 'running');
CREATE INDEX runs_task_id_created_at_idx ON runs (task_id, created_at DESC);
CREATE INDEX runs_status_idx ON runs (status);

-- +goose Down
DROP TABLE runs;
