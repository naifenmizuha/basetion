-- name: CreateTeam :exec
INSERT INTO teams (id, name, active, created_at, updated_at, deleted_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetTeam :one
SELECT id::text, name, active, created_at, updated_at, deleted_at
FROM teams
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateTeam :execrows
UPDATE teams
SET name = $1,
    active = $2,
    updated_at = $3,
    deleted_at = $4
WHERE id = $5 AND deleted_at IS NULL;

-- name: ListTeams :many
SELECT id::text, name, active, created_at, updated_at, deleted_at
FROM teams
WHERE deleted_at IS NULL
  AND (NOT sqlc.arg(active_only)::boolean OR active)
ORDER BY name, id;
