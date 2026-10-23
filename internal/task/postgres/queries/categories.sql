-- name: InsertCategory :exec
INSERT INTO project_categories (id, name, slug, description, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetCategory :one
SELECT id, name, slug, description, created_at, updated_at
FROM project_categories
WHERE id = $1;

-- name: GetCategoryForUpdate :one
SELECT id, name, slug, description, created_at, updated_at
FROM project_categories
WHERE id = $1
FOR UPDATE;

-- name: ListCategories :many
SELECT id, name, slug, description, created_at, updated_at
FROM project_categories
ORDER BY name, id;

-- name: UpdateCategory :exec
UPDATE project_categories
SET name = $2, slug = $3, description = $4, updated_at = $5
WHERE id = $1;

-- name: DeleteCategory :execrows
DELETE FROM project_categories WHERE id = $1;
