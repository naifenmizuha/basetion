INSERT INTO teams(id, name, active, version, created_at, updated_at)
VALUES ('00000000-0000-0000-0000-000000000001', 'Basetion Demo', TRUE, 1, now(), now());

INSERT INTO players(id, name, batting_flags, throwing_flags, position_flags, active, version, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0000-000000000011', '林一郎', 1, 2, 65, TRUE, 1, now(), now()),
    ('00000000-0000-0000-0000-000000000012', '陈捕手', 2, 2, 2, TRUE, 1, now(), now());

INSERT INTO memberships(id, team_id, player_id, jersey_number, joined_at, left_at, version, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0000-000000000021', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000011', 18, '2026-01-01', NULL, 1, now(), now()),
    ('00000000-0000-0000-0000-000000000022', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000012', 2, '2026-01-01', NULL, 1, now(), now());
