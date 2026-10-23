-- name: UpsertLabels :exec
INSERT INTO labels (id, name)
SELECT unnest(sqlc.arg('ids')::uuid[]), unnest(sqlc.arg('names')::text[])
ON CONFLICT (name) DO NOTHING;

-- name: DeleteTaskLabels :exec
DELETE FROM task_labels WHERE task_id = $1;

-- name: InsertTaskLabels :exec
INSERT INTO task_labels (task_id, label_id)
SELECT sqlc.arg('task_id')::uuid, id FROM labels WHERE name = ANY(sqlc.arg('names')::text[]);

-- name: ListTaskLabels :many
SELECT tl.task_id, l.name
FROM task_labels tl
JOIN labels l ON l.id = tl.label_id
WHERE tl.task_id = ANY(sqlc.arg('task_ids')::uuid[])
ORDER BY l.name;

-- name: ListLabels :many
SELECT name FROM labels ORDER BY name;
