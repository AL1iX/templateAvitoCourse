-- +goose Up
CREATE TABLE idempotency_keys
(
    key           UUID PRIMARY KEY,
    request_hash  TEXT        NOT NULL,
    response_body JSONB       NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE idempotency_keys;
