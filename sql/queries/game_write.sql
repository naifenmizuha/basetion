-- name: CreateMatch :exec
INSERT INTO matches (id, home_team_id, away_team_id, scheduled_at, location, status, created_at, updated_at, deleted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetMatch :one
SELECT id::text, home_team_id::text, away_team_id::text, scheduled_at, location, status, created_at, updated_at, deleted_at
FROM matches
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateMatch :execrows
UPDATE matches
SET home_team_id = $1, away_team_id = $2, scheduled_at = $3, location = $4, status = $5,
    updated_at = $6, deleted_at = $7
WHERE id = $8 AND deleted_at IS NULL;

-- name: ListStoredMatches :many
SELECT id::text, home_team_id::text, away_team_id::text, scheduled_at, location, status, created_at, updated_at, deleted_at
FROM matches
WHERE deleted_at IS NULL
ORDER BY scheduled_at, id;

-- name: CreateLineupEntry :exec
INSERT INTO lineups (id, match_id, team_id, kind, variant_number, variant_name, player_id, batting_order, position, created_at, updated_at, deleted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: GetLineupEntries :many
SELECT id::text, player_id::text, batting_order, position, variant_name, created_at, updated_at, deleted_at
FROM lineups
WHERE match_id = $1 AND team_id = $2 AND kind = $3 AND variant_number = $4 AND deleted_at IS NULL
ORDER BY batting_order, id;

-- name: SoftDeleteLineup :execrows
UPDATE lineups SET deleted_at = $1, updated_at = $1
WHERE match_id = $2 AND team_id = $3 AND kind = $4 AND variant_number = $5 AND deleted_at IS NULL;

-- name: SoftDeleteLineupsByMatch :exec
UPDATE lineups SET deleted_at = $1, updated_at = $1
WHERE match_id = $2 AND deleted_at IS NULL;

-- name: ListLineupKeysByMatch :many
SELECT DISTINCT team_id::text, kind, variant_number
FROM lineups
WHERE match_id = $1 AND deleted_at IS NULL
ORDER BY 1, 2, 3;

-- name: LineupNameOccupied :one
SELECT EXISTS(
  SELECT 1 FROM lineups
  WHERE match_id = $1 AND team_id = $2 AND variant_name = $3 AND deleted_at IS NULL
    AND NOT (kind = $4 AND variant_number = $5)
);
