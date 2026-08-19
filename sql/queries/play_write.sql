-- name: CreatePlay :exec
INSERT INTO plays (id, match_id, sequence, inning, half, batting_order, batter_id, starting_pitcher_id, batting_result, result_description, before_outs, before_home_score, before_away_score, before_runner_on_first_id, before_runner_on_second_id, before_runner_on_third_id, after_outs, after_home_score, after_away_score, after_runner_on_first_id, after_runner_on_second_id, after_runner_on_third_id, created_at, updated_at, deleted_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25);

-- name: UpdatePlay :execrows
UPDATE plays SET match_id=$1, sequence=$2, inning=$3, half=$4, batting_order=$5, batter_id=$6, starting_pitcher_id=$7, batting_result=$8, result_description=$9,
 before_outs=$10, before_home_score=$11, before_away_score=$12, before_runner_on_first_id=$13, before_runner_on_second_id=$14, before_runner_on_third_id=$15,
 after_outs=$16, after_home_score=$17, after_away_score=$18, after_runner_on_first_id=$19, after_runner_on_second_id=$20, after_runner_on_third_id=$21,
 created_at=$22, updated_at=$23, deleted_at=$24
WHERE id=$25 AND deleted_at IS NULL;

-- name: CreatePlayPitch :exec
INSERT INTO play_pitching_results (id, play_id, sequence, pitcher_id, batter_id, result, balls_before, strikes_before, balls_after, strikes_after, pitch_type, velocity, zone, description, created_at, updated_at, deleted_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17);

-- name: CreatePlayRunnerOutcome :exec
INSERT INTO play_runner_results (id, play_id, sequence, runner_id, result, from_base, to_base, out_recorded, scored, charged_pitcher_id, earned, rbi_batter_id, description, created_at, updated_at, deleted_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16);

-- name: CreatePlayFieldingOutcome :exec
INSERT INTO play_fielding_results (id, play_id, sequence, fielder_id, position, result, description, created_at, updated_at, deleted_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10);

-- name: SoftDeletePlayPitches :exec
UPDATE play_pitching_results SET deleted_at=$1, updated_at=$1 WHERE play_id=$2 AND deleted_at IS NULL;

-- name: SoftDeletePlayRunnerOutcomes :exec
UPDATE play_runner_results SET deleted_at=$1, updated_at=$1 WHERE play_id=$2 AND deleted_at IS NULL;

-- name: SoftDeletePlayFieldingOutcomes :exec
UPDATE play_fielding_results SET deleted_at=$1, updated_at=$1 WHERE play_id=$2 AND deleted_at IS NULL;

-- name: SoftDeletePlay :execrows
UPDATE plays SET deleted_at=$1, updated_at=$1 WHERE id=$2 AND deleted_at IS NULL;

-- name: SoftDeletePlaysByMatch :exec
UPDATE plays SET deleted_at=$1, updated_at=$1 WHERE match_id=$2 AND deleted_at IS NULL;
