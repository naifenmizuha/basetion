CREATE TABLE teams (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL,
    version BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE players (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    batting_flags SMALLINT NOT NULL,
    throwing_flags SMALLINT NOT NULL,
    position_flags SMALLINT NOT NULL,
    active BOOLEAN NOT NULL,
    version BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE memberships (
    id UUID PRIMARY KEY,
    team_id UUID NOT NULL,
    player_id UUID NOT NULL,
    jersey_number SMALLINT NOT NULL,
    joined_at DATE NOT NULL,
    left_at DATE,
    version BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_teams_active ON teams(active);
CREATE INDEX idx_players_active ON players(active);
CREATE INDEX idx_memberships_team_active ON memberships(team_id, left_at);
CREATE INDEX idx_memberships_team_jersey ON memberships(team_id, jersey_number);
CREATE INDEX idx_memberships_team_player ON memberships(team_id, player_id);
CREATE INDEX idx_memberships_player ON memberships(player_id);
CREATE INDEX idx_memberships_period ON memberships(team_id, joined_at, left_at);
