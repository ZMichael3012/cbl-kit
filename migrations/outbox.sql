-- Outbox-таблица платформы CBL: reference DDL из .specs/CBL-003-contracts.md.
-- Копируется в сервис как goose-миграция; AutoMigrate GORM запрещен.
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

-- Partial-индекс ускоряет polling неопубликованных событий.
CREATE INDEX IF NOT EXISTS outbox_pending_idx ON outbox (id) WHERE published_at IS NULL;
