CREATE UNIQUE INDEX uq_teams_active_name
ON teams(name)
WHERE deleted_at IS NULL;
