CREATE TABLE training_records (
    id UUID PRIMARY KEY,
    player_id UUID NOT NULL,
    training_date DATE NOT NULL,
    content TEXT NOT NULL,
    reflection TEXT NOT NULL,
    version BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_training_records_active_player_date
    ON training_records(player_id, training_date)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_training_records_active_player_date
    ON training_records(player_id, training_date DESC, id)
    WHERE deleted_at IS NULL;
