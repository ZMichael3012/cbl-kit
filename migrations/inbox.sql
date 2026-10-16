-- Inbox table of the CBL platform: reference DDL from .specs/CBL-003-contracts.md.
-- Copied into a service as a goose migration; GORM AutoMigrate is forbidden.
CREATE TABLE IF NOT EXISTS inbox (
    consumer     text        NOT NULL,
    event_id     uuid        NOT NULL,
    processed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);
