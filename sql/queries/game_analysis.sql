-- name: ListOffenseLines :many
WITH x AS (
 SELECT p.match_id,p.batter_id,COUNT(*)::int pa,COUNT(*) FILTER (WHERE p.batting_result NOT IN (5,6,7,14,15,16))::int ab,COUNT(*) FILTER (WHERE p.batting_result BETWEEN 1 AND 4)::int h,
 COUNT(*) FILTER (WHERE p.batting_result=1)::int singles,COUNT(*) FILTER (WHERE p.batting_result=2)::int doubles,COUNT(*) FILTER (WHERE p.batting_result=3)::int triples,COUNT(*) FILTER (WHERE p.batting_result=4)::int homers,
 COUNT(*) FILTER (WHERE p.batting_result IN(5,6))::int walks,COUNT(*) FILTER (WHERE p.batting_result=7)::int hbp,COUNT(*) FILTER (WHERE p.batting_result=8)::int so,
 SUM(CASE p.batting_result WHEN 1 THEN 1 WHEN 2 THEN 2 WHEN 3 THEN 3 WHEN 4 THEN 4 ELSE 0 END)::int tb
 FROM plays p WHERE p.match_id=ANY(sqlc.arg(match_ids)::uuid[]) AND p.deleted_at IS NULL GROUP BY p.match_id,p.batter_id
), r AS (
 SELECT p.match_id,ro.rbi_batter_id id,COUNT(*)::int rbi FROM play_runner_results ro JOIN plays p ON p.id=ro.play_id AND p.deleted_at IS NULL WHERE p.match_id=ANY(sqlc.arg(match_ids)::uuid[]) AND ro.deleted_at IS NULL AND ro.scored AND ro.rbi_batter_id IS NOT NULL GROUP BY p.match_id,ro.rbi_batter_id
), runs AS (
 SELECT p.match_id,ro.runner_id id,COUNT(*)::int runs FROM play_runner_results ro JOIN plays p ON p.id=ro.play_id AND p.deleted_at IS NULL WHERE p.match_id=ANY(sqlc.arg(match_ids)::uuid[]) AND ro.deleted_at IS NULL AND ro.scored GROUP BY p.match_id,ro.runner_id
)
SELECT x.match_id::text AS match_id,pl.name,t.name AS team_name,pl.jersey_number,x.pa,x.ab,x.h,x.singles,x.doubles,x.triples,x.homers,x.walks,x.hbp,x.so,COALESCE(r.rbi,0)::int AS rbi,COALESCE(runs.runs,0)::int AS runs,x.tb
FROM x JOIN players pl ON pl.id=x.batter_id AND pl.deleted_at IS NULL JOIN teams t ON t.id=pl.team_id AND t.deleted_at IS NULL LEFT JOIN r ON r.match_id=x.match_id AND r.id=x.batter_id LEFT JOIN runs ON runs.match_id=x.match_id AND runs.id=x.batter_id ORDER BY x.match_id,pl.name,pl.jersey_number;

-- name: ListPitchingLines :many
WITH attrib AS (
 SELECT p.match_id,p.id,p.batting_result,COALESCE((SELECT pr.pitcher_id FROM play_pitching_results pr WHERE pr.play_id=p.id AND pr.deleted_at IS NULL ORDER BY pr.sequence DESC LIMIT 1),p.starting_pitcher_id) pitcher,p.before_outs,p.after_outs
 FROM plays p WHERE p.match_id=ANY(sqlc.arg(match_ids)::uuid[]) AND p.deleted_at IS NULL
), base AS (
 SELECT match_id,pitcher,COUNT(*)::int bf,COUNT(*) FILTER(WHERE batting_result BETWEEN 1 AND 4)::int h,COUNT(*) FILTER(WHERE batting_result=4)::int hr,COUNT(*) FILTER(WHERE batting_result IN(5,6))::int bb,COUNT(*) FILTER(WHERE batting_result=7)::int hbp,COUNT(*) FILTER(WHERE batting_result=8)::int so,SUM(GREATEST(after_outs-before_outs,0))::int outs FROM attrib GROUP BY match_id,pitcher
), pitches AS (
 SELECT p.match_id,pr.pitcher_id id,COUNT(*)::int pitches,COUNT(*) FILTER(WHERE pr.result=2)::int called,COUNT(*) FILTER(WHERE pr.result=3)::int swinging FROM play_pitching_results pr JOIN plays p ON p.id=pr.play_id AND p.deleted_at IS NULL WHERE p.match_id=ANY(sqlc.arg(match_ids)::uuid[]) AND pr.deleted_at IS NULL GROUP BY p.match_id,pr.pitcher_id
), runs AS (
 SELECT p.match_id,ro.charged_pitcher_id id,COUNT(*)::int runs,COUNT(*) FILTER(WHERE ro.earned)::int er FROM play_runner_results ro JOIN plays p ON p.id=ro.play_id AND p.deleted_at IS NULL WHERE p.match_id=ANY(sqlc.arg(match_ids)::uuid[]) AND ro.deleted_at IS NULL AND ro.scored AND ro.charged_pitcher_id IS NOT NULL GROUP BY p.match_id,ro.charged_pitcher_id
)
SELECT b.match_id::text AS match_id,pl.name,t.name AS team_name,pl.jersey_number,b.bf,COALESCE(pi.pitches,0)::int AS pitches,COALESCE(pi.called,0)::int AS called_strikes,COALESCE(pi.swinging,0)::int AS swinging_strikes,b.h,b.hr,b.bb,b.hbp,b.so,COALESCE(r.runs,0)::int AS runs,COALESCE(r.er,0)::int AS earned_runs,b.outs
FROM base b JOIN players pl ON pl.id=b.pitcher AND pl.deleted_at IS NULL JOIN teams t ON t.id=pl.team_id AND t.deleted_at IS NULL LEFT JOIN pitches pi ON pi.match_id=b.match_id AND pi.id=b.pitcher LEFT JOIN runs r ON r.match_id=b.match_id AND r.id=b.pitcher ORDER BY b.match_id,pl.name,pl.jersey_number;

-- name: ListFieldingLines :many
SELECT p.match_id::text,pl.name,t.name AS team_name,pl.jersey_number,COUNT(*) FILTER(WHERE f.result=1)::int AS putouts,COUNT(*) FILTER(WHERE f.result=2)::int AS assists,COUNT(*) FILTER(WHERE f.result=3)::int AS errors
FROM play_fielding_results f JOIN plays p ON p.id=f.play_id AND p.deleted_at IS NULL JOIN players pl ON pl.id=f.fielder_id AND pl.deleted_at IS NULL JOIN teams t ON t.id=pl.team_id AND t.deleted_at IS NULL
WHERE p.match_id=ANY(sqlc.arg(match_ids)::uuid[]) AND f.deleted_at IS NULL GROUP BY p.match_id,pl.id,pl.name,t.name,pl.jersey_number ORDER BY p.match_id,pl.name,pl.jersey_number;
