DROP TABLE IF EXISTS plates;

CREATE TABLE plays (
    id UUID PRIMARY KEY, match_id UUID NOT NULL, sequence INTEGER NOT NULL,
    inning SMALLINT NOT NULL, half SMALLINT NOT NULL, batting_order SMALLINT NOT NULL,
    batter_id UUID NOT NULL, starting_pitcher_id UUID NOT NULL,
    batting_result SMALLINT NOT NULL, result_description TEXT NOT NULL,
    before_outs SMALLINT NOT NULL, before_home_score INTEGER NOT NULL, before_away_score INTEGER NOT NULL,
    before_runner_on_first_id UUID, before_runner_on_second_id UUID, before_runner_on_third_id UUID,
    after_outs SMALLINT NOT NULL, after_home_score INTEGER NOT NULL, after_away_score INTEGER NOT NULL,
    after_runner_on_first_id UUID, after_runner_on_second_id UUID, after_runner_on_third_id UUID,
    version BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL, deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_plays_active_sequence ON plays(match_id, sequence) WHERE deleted_at IS NULL;
CREATE INDEX idx_plays_match ON plays(match_id, sequence) WHERE deleted_at IS NULL;

CREATE TABLE pitches (
    id UUID PRIMARY KEY, play_id UUID NOT NULL, sequence SMALLINT NOT NULL,
    pitcher_id UUID NOT NULL, batter_id UUID NOT NULL, result SMALLINT NOT NULL,
    balls_before SMALLINT NOT NULL, strikes_before SMALLINT NOT NULL,
    balls_after SMALLINT NOT NULL, strikes_after SMALLINT NOT NULL,
    pitch_type TEXT NOT NULL, velocity DOUBLE PRECISION, zone SMALLINT, description TEXT NOT NULL,
    version BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL, deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_pitches_active_sequence ON pitches(play_id, sequence) WHERE deleted_at IS NULL;

CREATE TABLE play_runner_results (
    id UUID PRIMARY KEY, play_id UUID NOT NULL, sequence SMALLINT NOT NULL,
    runner_id UUID NOT NULL, result SMALLINT NOT NULL, from_base SMALLINT NOT NULL, to_base SMALLINT,
    out_recorded BOOLEAN NOT NULL, scored BOOLEAN NOT NULL, charged_pitcher_id UUID,
    earned BOOLEAN, rbi_batter_id UUID, description TEXT NOT NULL,
    version BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL, deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_play_runner_results_active_sequence ON play_runner_results(play_id, sequence) WHERE deleted_at IS NULL;

CREATE TABLE play_fielding_results (
    id UUID PRIMARY KEY, play_id UUID NOT NULL, sequence SMALLINT NOT NULL,
    fielder_id UUID NOT NULL, position SMALLINT NOT NULL, result SMALLINT NOT NULL, description TEXT NOT NULL,
    version BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL, deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_play_fielding_results_active_sequence ON play_fielding_results(play_id, sequence) WHERE deleted_at IS NULL;
