# cbl-kit

Общая Go-библиотека платформы CBL: денежный тип, HMAC-подпись, middleware Gin и gRPC,
транзакционный outbox и идемпотентный inbox.

Правила репозитория и конвенции: `AGENTS.md`, глобальные правила `../../AGENTS.md`.
Спецификации задач: `.specs/CBL-1.*.md`.

## Пакеты

| Пакет | Назначение |
| :-- | :-- |
| `money` | Денежный тип int64 + decimals, float запрещен |
| `hmac` | Подпись вебхуков по сырому телу, окно replay |
| `ginmw` | request_id, problem+json, access log, recovery |
| `grpcmw` | интерсепторы и маппинг доменных ошибок |
| `outbox` | запись события в транзакции, relay, kafkapub |
| `inbox` | Guard идемпотентности, retry/DLQ, kafkaconsumer |

## Команды

```powershell
go build ./...
go vet ./...
go test ./... -cover
go test -tags integration ./outbox/... ./inbox/...   # требует Docker (testcontainers)
```

## Миграции

`migrations/outbox.sql` и `migrations/inbox.sql` - reference DDL из
`.specs/CBL-003-contracts.md`, копируются в сервисы как goose-миграции.
`AutoMigrate` GORM запрещен.
