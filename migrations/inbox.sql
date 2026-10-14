-- Inbox-таблица платформы CBL: reference DDL из .specs/CBL-003-contracts.md.
-- Копируется в сервис как goose-миграция; AutoMigrate GORM запрещен.
CREATE TABLE IF NOT EXISTS inbox (
    consumer     text        NOT NULL,
    event_id     uuid        NOT NULL,
    processed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);
