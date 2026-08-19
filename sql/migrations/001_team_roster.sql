CREATE TABLE teams (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE players (
    id UUID PRIMARY KEY,
    team_id UUID NOT NULL,
    jersey_number SMALLINT NOT NULL CHECK (jersey_number BETWEEN 0 AND 99),
    name TEXT NOT NULL,
    batting_flags SMALLINT NOT NULL,
    throwing_flags SMALLINT NOT NULL,
    position_flags SMALLINT NOT NULL,
    active BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_teams_active ON teams(active);
CREATE INDEX idx_players_active ON players(active);
