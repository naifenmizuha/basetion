-- name: CreateTrainingRecord :exec
INSERT INTO training_records (id, player_id, training_date, content, reflection, created_at, updated_at, deleted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetTrainingRecord :one
SELECT id::text, player_id::text, training_date, content, reflection, created_at, updated_at, deleted_at
FROM training_records
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateTrainingRecord :execrows
UPDATE training_records
SET content = $1,
    reflection = $2,
    updated_at = $3,
    deleted_at = $4
WHERE id = $5 AND deleted_at IS NULL;

-- name: ListTrainingRecords :many
SELECT id::text, player_id::text, training_date, content, reflection, created_at, updated_at, deleted_at
FROM training_records
WHERE player_id = $1
  AND deleted_at IS NULL
  AND ($2::date IS NULL OR training_date >= $2)
  AND ($3::date IS NULL OR training_date <= $3)
ORDER BY training_date DESC, id;

-- name: SoftDeleteTrainingRecordsByPlayer :exec
UPDATE training_records
SET deleted_at = $1,
    updated_at = $1
WHERE player_id = $2 AND deleted_at IS NULL;
