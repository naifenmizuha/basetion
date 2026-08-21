-- name: CreatePlayer :exec
INSERT INTO players (id, team_id, jersey_number, name, batting_flags, throwing_flags, position_flags, active, created_at, updated_at, deleted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: GetPlayer :one
SELECT id::text, team_id::text, jersey_number, name, batting_flags, throwing_flags, position_flags, active, created_at, updated_at, deleted_at
FROM players
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetPlayersByTeamAndJerseys :many
WITH requested AS (
    SELECT
        split_part(value, '/', 1)::uuid AS team_id,
        split_part(value, '/', 2)::smallint AS jersey_number
    FROM unnest(sqlc.arg(keys)::text[]) AS requested(value)
)
SELECT p.id::text AS id, p.team_id::text AS team_id, p.jersey_number, p.name, p.batting_flags, p.throwing_flags, p.position_flags, p.active, p.created_at, p.updated_at, p.deleted_at
FROM requested r
JOIN players p ON p.team_id = r.team_id AND p.jersey_number = r.jersey_number
WHERE p.deleted_at IS NULL
ORDER BY p.team_id, p.jersey_number;

-- name: GetPlayersByName :many
SELECT id::text, team_id::text, jersey_number, name, batting_flags, throwing_flags, position_flags, active, created_at, updated_at, deleted_at
FROM players
WHERE name = $1 AND deleted_at IS NULL
  AND (sqlc.narg(team_id)::uuid IS NULL OR team_id = sqlc.narg(team_id)::uuid)
ORDER BY created_at, id;

-- name: GetPlayersByIDs :many
SELECT id::text, team_id::text, jersey_number, name, batting_flags, throwing_flags, position_flags, active, created_at, updated_at, deleted_at
FROM players
WHERE id = ANY(sqlc.arg(ids)::uuid[]) AND deleted_at IS NULL
ORDER BY created_at, id;

-- name: UpdatePlayer :execrows
UPDATE players
SET jersey_number = $1,
    name = $2,
    batting_flags = $3,
    throwing_flags = $4,
    position_flags = $5,
    active = $6,
    updated_at = $7,
    deleted_at = $8
WHERE id = $9 AND deleted_at IS NULL;

-- name: ListPlayers :many
SELECT p.name, t.name, p.jersey_number, p.position_flags, p.active
FROM players p
JOIN teams t ON t.id = p.team_id AND t.deleted_at IS NULL
WHERE p.deleted_at IS NULL
  AND (sqlc.narg(team_name)::text IS NULL OR t.name = sqlc.narg(team_name)::text)
  AND (sqlc.narg(player_name)::text IS NULL OR p.name = sqlc.narg(player_name)::text)
ORDER BY t.name, p.jersey_number, p.name, p.id;

-- name: PlayerJerseyOccupied :one
SELECT EXISTS(
    SELECT 1
    FROM players
WHERE team_id = sqlc.arg(team_id)
      AND jersey_number = sqlc.arg(jersey_number)
      AND deleted_at IS NULL
      AND (sqlc.narg(except_player_id)::uuid IS NULL OR id <> sqlc.narg(except_player_id)::uuid)
);

-- name: SoftDeletePlayersByTeam :exec
UPDATE players
SET deleted_at = $1,
    updated_at = $1
WHERE team_id = $2 AND deleted_at IS NULL;

-- name: ListPlayerViews :many
SELECT p.name, t.name AS team_name, p.jersey_number, p.position_flags, p.active
FROM players p JOIN teams t ON t.id=p.team_id
WHERE p.deleted_at IS NULL AND t.deleted_at IS NULL
  AND (sqlc.arg(team_name)::text='' OR t.name=sqlc.arg(team_name)::text)
  AND (sqlc.arg(position_any)::smallint=0 OR (p.position_flags & sqlc.arg(position_any)::smallint)<>0)
  AND (sqlc.narg(jersey_number)::smallint IS NULL OR p.jersey_number=sqlc.narg(jersey_number)::smallint)
ORDER BY p.jersey_number,p.name,p.id;

-- name: AuditOrphanTeams :many
SELECT p.team_id::text AS team_id, p.id::text AS player_id, p.jersey_number, 1::int AS issue_count
FROM players p LEFT JOIN teams t ON t.id=p.team_id AND t.deleted_at IS NULL
WHERE p.deleted_at IS NULL AND t.id IS NULL;

-- name: AuditDuplicateJerseys :many
SELECT team_id::text AS team_id, ''::text AS player_id, jersey_number, count(*)::int AS issue_count
FROM players WHERE deleted_at IS NULL GROUP BY team_id,jersey_number HAVING count(*)>1;
