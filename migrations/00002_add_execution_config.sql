-- +goose Up
ALTER TABLE tasks
    ADD COLUMN agent  text,
    ADD COLUMN model  text,
    ADD COLUMN effort text,
    ADD COLUMN runner text;

ALTER TABLE project_categories
    ADD COLUMN repo_url    text,
    ADD COLUMN base_branch text NOT NULL DEFAULT 'main';

-- +goose Down
ALTER TABLE project_categories
    DROP COLUMN base_branch,
    DROP COLUMN repo_url;

ALTER TABLE tasks
    DROP COLUMN runner,
    DROP COLUMN effort,
    DROP COLUMN model,
    DROP COLUMN agent;
