-- +goose Up
CREATE TABLE project_categories (
    id          uuid        PRIMARY KEY,
    name        text        NOT NULL,
    slug        text        NOT NULL UNIQUE,
    description text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL
);

CREATE TABLE tasks (
    id                      uuid        PRIMARY KEY,
    provider                text        NOT NULL,
    external_id             text        NOT NULL,
    external_url            text,
    title                   text        NOT NULL,
    description             text        NOT NULL,
    additional_instructions text[]      NOT NULL DEFAULT '{}',
    project_category_id     uuid        REFERENCES project_categories (id) ON DELETE RESTRICT,
    status                  text        NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'ready', 'in_progress', 'in_review', 'done', 'failed', 'cancelled')),
    raw_payload             jsonb,
    created_at              timestamptz NOT NULL,
    updated_at              timestamptz NOT NULL,
    UNIQUE (provider, external_id)
);

CREATE INDEX tasks_status_idx ON tasks (status);
CREATE INDEX tasks_project_category_id_idx ON tasks (project_category_id);
CREATE INDEX tasks_created_at_idx ON tasks (created_at DESC);

CREATE TABLE labels (
    id   uuid PRIMARY KEY,
    name text NOT NULL UNIQUE
);

CREATE TABLE task_labels (
    task_id  uuid NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    label_id uuid NOT NULL REFERENCES labels (id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, label_id)
);

CREATE INDEX task_labels_label_id_idx ON task_labels (label_id);

-- +goose Down
DROP TABLE task_labels;
DROP TABLE labels;
DROP TABLE tasks;
DROP TABLE project_categories;
