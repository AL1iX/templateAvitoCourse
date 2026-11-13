-- +goose NO TRANSACTION

-- +goose Up
CREATE INDEX CONCURRENTLY trips_active_last_activity_idx
    ON trips ((COALESCE(last_position_at, started_at)))
    WHERE status = 'active';

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS trips_active_last_activity_idx;
