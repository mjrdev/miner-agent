-- name: LockTaskStatus :one
SELECT status FROM tasks WHERE id = $1 FOR UPDATE;

-- name: MoveTask :execrows
UPDATE tasks
SET status = sqlc.arg('to'), updated_at = sqlc.arg('updated_at')
WHERE id = sqlc.arg('id') AND status = sqlc.arg('from');

-- name: InsertRun :exec
INSERT INTO runs (
    id, task_id, status, agent, model, effort, runner, repo_url, base_branch,
    branch, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
);

-- name: DeleteQueuedRun :one
DELETE FROM runs WHERE id = $1 AND status = 'queued' RETURNING task_id;

-- name: GetRun :one
SELECT * FROM runs WHERE id = $1;

-- name: ListRunsByTask :many
SELECT
    id, task_id, status, agent, model, effort, runner, repo_url, base_branch,
    branch, auth_method, effective_model, cost_usd, num_turns, exit_code, error,
    cancel_requested_at, created_at, started_at, finished_at
FROM runs
WHERE task_id = $1
ORDER BY created_at DESC, id DESC;

-- name: GetActiveRun :one
SELECT * FROM runs WHERE task_id = $1 AND status IN ('queued', 'running');

-- name: MarkRunning :execrows
UPDATE runs
SET status = 'running', started_at = $2, auth_method = $3
WHERE id = $1 AND status = 'queued';

-- name: RequestCancel :execrows
UPDATE runs
SET cancel_requested_at = COALESCE(cancel_requested_at, $2)
WHERE id = $1 AND status = 'running';

-- name: FinishRun :one
UPDATE runs
SET status = $2,
    effective_model = $3,
    cost_usd = $4,
    num_turns = $5,
    exit_code = $6,
    error = $7,
    diff = $8,
    finished_at = $9
WHERE id = $1 AND status IN ('queued', 'running')
RETURNING task_id;

-- name: ListRunningRuns :many
SELECT * FROM runs WHERE status = 'running' ORDER BY started_at;
