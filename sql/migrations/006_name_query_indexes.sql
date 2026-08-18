CREATE INDEX idx_teams_active_name ON teams(name) WHERE deleted_at IS NULL;
CREATE INDEX idx_players_active_name ON players(name) WHERE deleted_at IS NULL;
