ALTER TABLE teams ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE players ADD COLUMN deleted_at TIMESTAMPTZ;

CREATE UNIQUE INDEX uq_players_active_team_jersey ON players(team_id, jersey_number) WHERE deleted_at IS NULL;
CREATE INDEX idx_players_active_team_jersey_name ON players(team_id, jersey_number, name, id) WHERE deleted_at IS NULL;
CREATE INDEX idx_players_active_name ON players(name, jersey_number, id) WHERE deleted_at IS NULL;

CREATE TABLE matches (
    id UUID PRIMARY KEY,
    home_team_id UUID NOT NULL,
    away_team_id UUID NOT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL,
    location TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ
);

CREATE TABLE lineups (
    id UUID PRIMARY KEY,
    match_id UUID NOT NULL,
    team_id UUID NOT NULL,
    kind SMALLINT NOT NULL,
    variant_number SMALLINT NOT NULL,
    variant_name TEXT NOT NULL,
    player_id UUID NOT NULL,
    batting_order SMALLINT NOT NULL,
    position SMALLINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_matches_scheduled ON matches(scheduled_at, id) WHERE deleted_at IS NULL;
CREATE INDEX idx_lineups_match ON lineups(match_id, team_id, kind, variant_number, batting_order) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_lineups_active_player ON lineups(match_id, team_id, kind, variant_number, player_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_lineups_active_order ON lineups(match_id, team_id, kind, variant_number, batting_order) WHERE deleted_at IS NULL;
