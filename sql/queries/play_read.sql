-- name: GetPlay :one
SELECT id::text, match_id::text, sequence, inning, half, batting_order, batter_id::text, starting_pitcher_id::text, batting_result, result_description,
       before_outs, before_home_score, before_away_score, COALESCE(before_runner_on_first_id::text, ''::text) AS before_runner_on_first_id, COALESCE(before_runner_on_second_id::text, ''::text) AS before_runner_on_second_id, COALESCE(before_runner_on_third_id::text, ''::text) AS before_runner_on_third_id,
       after_outs, after_home_score, after_away_score, COALESCE(after_runner_on_first_id::text, ''::text) AS after_runner_on_first_id, COALESCE(after_runner_on_second_id::text, ''::text) AS after_runner_on_second_id, COALESCE(after_runner_on_third_id::text, ''::text) AS after_runner_on_third_id,
       created_at, updated_at, deleted_at
FROM plays WHERE id=$1 AND deleted_at IS NULL;

-- name: ListPlayIDsByMatch :many
SELECT id::text FROM plays WHERE match_id=$1 AND deleted_at IS NULL ORDER BY sequence;

-- name: ListPlayPitches :many
SELECT id::text, sequence, pitcher_id::text, batter_id::text, result, balls_before, strikes_before, balls_after, strikes_after, pitch_type, velocity, zone, description, created_at, updated_at, deleted_at
FROM play_pitching_results WHERE play_id=$1 AND deleted_at IS NULL ORDER BY sequence;

-- name: ListPlayRunnerOutcomes :many
SELECT id::text, sequence, runner_id::text, result, from_base, to_base, out_recorded, scored, COALESCE(charged_pitcher_id::text, ''::text) AS charged_pitcher_id, earned, COALESCE(rbi_batter_id::text, ''::text) AS rbi_batter_id, description, created_at, updated_at, deleted_at
FROM play_runner_results WHERE play_id=$1 AND deleted_at IS NULL ORDER BY sequence;

-- name: ListPlayFieldingOutcomes :many
SELECT id::text, sequence, fielder_id::text, position, result, description, created_at, updated_at, deleted_at
FROM play_fielding_results WHERE play_id=$1 AND deleted_at IS NULL ORDER BY sequence;
