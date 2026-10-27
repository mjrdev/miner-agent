-- name: InsertCategory :exec
INSERT INTO project_categories (id, name, slug, description, repo_url, base_branch, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetCategory :one
SELECT id, name, slug, description, created_at, updated_at, repo_url, base_branch
FROM project_categories
WHERE id = $1;

-- name: GetCategoryForUpdate :one
SELECT id, name, slug, description, created_at, updated_at, repo_url, base_branch
FROM project_categories
WHERE id = $1
FOR UPDATE;

-- name: ListCategories :many
SELECT id, name, slug, description, created_at, updated_at, repo_url, base_branch
FROM project_categories
ORDER BY name, id;

-- name: UpdateCategory :exec
UPDATE project_categories
SET name = $2, slug = $3, description = $4, repo_url = $5, base_branch = $6, updated_at = $7
WHERE id = $1;

-- name: DeleteCategory :execrows
DELETE FROM project_categories WHERE id = $1;
