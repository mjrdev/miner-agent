-- name: InsertTask :exec
INSERT INTO tasks (
    id, provider, external_id, external_url, title, description,
    additional_instructions, project_category_id, status, raw_payload,
    agent, model, effort, runner, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
);

-- name: InsertTaskIfAbsent :execrows
INSERT INTO tasks (
    id, provider, external_id, external_url, title, description,
    additional_instructions, project_category_id, status, raw_payload,
    agent, model, effort, runner, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
)
ON CONFLICT (provider, external_id) DO NOTHING;

-- name: GetTask :one
SELECT
    t.id, t.provider, t.external_id, t.external_url, t.title, t.description,
    t.additional_instructions, t.project_category_id, t.status,
    t.agent, t.model, t.effort, t.runner, t.created_at, t.updated_at,
    c.slug AS category_slug, c.name AS category_name, c.repo_url AS category_repo_url
FROM tasks t
LEFT JOIN project_categories c ON c.id = t.project_category_id
WHERE t.id = $1;

-- name: LockTask :one
SELECT id FROM tasks WHERE id = $1 FOR UPDATE;

-- name: ListTasks :many
SELECT
    t.id, t.provider, t.external_id, t.external_url, t.title, t.description,
    t.additional_instructions, t.project_category_id, t.status,
    t.agent, t.model, t.effort, t.runner, t.created_at, t.updated_at,
    c.slug AS category_slug, c.name AS category_name, c.repo_url AS category_repo_url
FROM tasks t
LEFT JOIN project_categories c ON c.id = t.project_category_id
WHERE (sqlc.narg('status')::text IS NULL OR t.status = sqlc.narg('status'))
  AND (sqlc.narg('provider')::text IS NULL OR t.provider = sqlc.narg('provider'))
  AND (sqlc.narg('project_category_id')::uuid IS NULL OR t.project_category_id = sqlc.narg('project_category_id'))
  AND (sqlc.narg('label')::text IS NULL OR EXISTS (
        SELECT 1 FROM task_labels tl
        JOIN labels l ON l.id = tl.label_id
        WHERE tl.task_id = t.id AND l.name = sqlc.narg('label')))
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountTasks :one
SELECT count(*)
FROM tasks t
WHERE (sqlc.narg('status')::text IS NULL OR t.status = sqlc.narg('status'))
  AND (sqlc.narg('provider')::text IS NULL OR t.provider = sqlc.narg('provider'))
  AND (sqlc.narg('project_category_id')::uuid IS NULL OR t.project_category_id = sqlc.narg('project_category_id'))
  AND (sqlc.narg('label')::text IS NULL OR EXISTS (
        SELECT 1 FROM task_labels tl
        JOIN labels l ON l.id = tl.label_id
        WHERE tl.task_id = t.id AND l.name = sqlc.narg('label')));

-- name: UpdateTask :exec
UPDATE tasks
SET title = $2,
    description = $3,
    additional_instructions = $4,
    project_category_id = $5,
    status = $6,
    agent = $7,
    model = $8,
    effort = $9,
    runner = $10,
    updated_at = $11
WHERE id = $1;

-- name: DeleteTask :execrows
DELETE FROM tasks WHERE id = $1;
