-- Outbox table of the CBL platform: reference DDL from .specs/CBL-003-contracts.md.
-- Copied into a service as a goose migration; GORM AutoMigrate is forbidden.
CREATE TABLE IF NOT EXISTS outbox (
    id           bigserial PRIMARY KEY,
    event_id     uuid        NOT NULL UNIQUE,
    topic        text        NOT NULL,
    key          text        NOT NULL,
    headers      jsonb       NOT NULL,
    payload      bytea       NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);

-- The partial index speeds up polling of unpublished events.
CREATE INDEX IF NOT EXISTS outbox_pending_idx ON outbox (id) WHERE published_at IS NULL;
