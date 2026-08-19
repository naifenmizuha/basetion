-- name: ListSelectedMatches :many
SELECT m.id::text, m.scheduled_at, home.name AS home_team_name, away.name AS away_team_name, m.location, m.status
FROM matches m JOIN teams home ON home.id=m.home_team_id JOIN teams away ON away.id=m.away_team_id
WHERE m.deleted_at IS NULL AND home.deleted_at IS NULL AND away.deleted_at IS NULL
  AND (COALESCE(cardinality(sqlc.arg(participant_names)::text[]),0)=0 OR home.name=ANY(sqlc.arg(participant_names)::text[]) OR away.name=ANY(sqlc.arg(participant_names)::text[]))
  AND (sqlc.narg(scheduled_from)::timestamptz IS NULL OR m.scheduled_at>=sqlc.narg(scheduled_from)::timestamptz)
  AND (sqlc.narg(scheduled_to)::timestamptz IS NULL OR m.scheduled_at<=sqlc.narg(scheduled_to)::timestamptz)
ORDER BY m.scheduled_at DESC,m.id DESC LIMIT NULLIF(sqlc.arg(row_limit)::int,0);

-- name: ListMatchFinalScores :many
SELECT DISTINCT ON (match_id) match_id::text, after_home_score, after_away_score
FROM plays
WHERE match_id = ANY(sqlc.arg(match_ids)::uuid[]) AND deleted_at IS NULL
ORDER BY match_id, sequence DESC;

-- name: ListMatchLineupRows :many
SELECT l.match_id::text,t.name AS team_name,l.kind,l.variant_number,l.variant_name,p.name AS player_name,p.jersey_number,l.batting_order,l.position,l.id::text AS entry_id
FROM lineups l JOIN teams t ON t.id=l.team_id AND t.deleted_at IS NULL JOIN players p ON p.id=l.player_id AND p.deleted_at IS NULL
WHERE l.match_id=ANY(sqlc.arg(match_ids)::uuid[]) AND l.deleted_at IS NULL
ORDER BY l.match_id,t.name,l.kind,l.variant_number,l.batting_order,l.id;

-- name: ListMatchRecordEvents :many
SELECT p.match_id::text,p.id::text AS play_id,p.sequence,p.inning,p.half,p.batting_order,
 b.name AS batter_name,bt.name AS batter_team_name,b.jersey_number AS batter_jersey_number,
 sp.name AS pitcher_name,spt.name AS pitcher_team_name,sp.jersey_number AS pitcher_jersey_number,
 p.after_outs,p.after_home_score,p.after_away_score,p.batting_result,p.result_description
FROM plays p JOIN players b ON b.id=p.batter_id AND b.deleted_at IS NULL JOIN teams bt ON bt.id=b.team_id AND bt.deleted_at IS NULL
 JOIN players sp ON sp.id=p.starting_pitcher_id AND sp.deleted_at IS NULL JOIN teams spt ON spt.id=sp.team_id AND spt.deleted_at IS NULL
WHERE p.match_id=ANY(sqlc.arg(match_ids)::uuid[]) AND p.deleted_at IS NULL
ORDER BY p.match_id,p.sequence;

-- name: ListRecordPitches :many
SELECT p.play_id::text,p.sequence,pi.name AS pitcher_name,pit.name AS pitcher_team_name,pi.jersey_number AS pitcher_jersey_number,
 b.name AS batter_name,bt.name AS batter_team_name,b.jersey_number AS batter_jersey_number,p.result,p.balls_after,p.strikes_after,p.pitch_type,p.velocity,p.zone,p.description
FROM play_pitching_results p JOIN players pi ON pi.id=p.pitcher_id AND pi.deleted_at IS NULL JOIN teams pit ON pit.id=pi.team_id AND pit.deleted_at IS NULL
 JOIN players b ON b.id=p.batter_id AND b.deleted_at IS NULL JOIN teams bt ON bt.id=b.team_id AND bt.deleted_at IS NULL
WHERE p.play_id=ANY(sqlc.arg(play_ids)::uuid[]) AND p.deleted_at IS NULL ORDER BY p.play_id,p.sequence;

-- name: ListRecordRunners :many
SELECT r.play_id::text,r.sequence,pl.name AS runner_name,pt.name AS runner_team_name,pl.jersey_number AS runner_jersey_number,r.result,r.from_base,r.to_base,r.out_recorded,r.scored,
 cp.name AS charged_pitcher_name,cpt.name AS charged_pitcher_team_name,cp.jersey_number AS charged_pitcher_jersey_number,r.earned,
 rb.name AS rbi_batter_name,rbt.name AS rbi_batter_team_name,rb.jersey_number AS rbi_batter_jersey_number,r.description
FROM play_runner_results r JOIN players pl ON pl.id=r.runner_id AND pl.deleted_at IS NULL JOIN teams pt ON pt.id=pl.team_id AND pt.deleted_at IS NULL
 LEFT JOIN players cp ON cp.id=r.charged_pitcher_id AND cp.deleted_at IS NULL LEFT JOIN teams cpt ON cpt.id=cp.team_id AND cpt.deleted_at IS NULL
 LEFT JOIN players rb ON rb.id=r.rbi_batter_id AND rb.deleted_at IS NULL LEFT JOIN teams rbt ON rbt.id=rb.team_id AND rbt.deleted_at IS NULL
WHERE r.play_id=ANY(sqlc.arg(play_ids)::uuid[]) AND r.deleted_at IS NULL ORDER BY r.play_id,r.sequence;

-- name: ListRecordFielding :many
SELECT f.play_id::text,f.sequence,p.name AS fielder_name,t.name AS fielder_team_name,p.jersey_number AS fielder_jersey_number,f.position,f.result,f.description
FROM play_fielding_results f JOIN players p ON p.id=f.fielder_id AND p.deleted_at IS NULL JOIN teams t ON t.id=p.team_id AND t.deleted_at IS NULL
WHERE f.play_id=ANY(sqlc.arg(play_ids)::uuid[]) AND f.deleted_at IS NULL ORDER BY f.play_id,f.sequence;
