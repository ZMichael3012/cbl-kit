// Пакет inbox реализует идемпотентную обработку Kafka-событий:
// Guard отсекает дубликаты по паре (consumer, event_id), Processor
// маршрутизирует ошибочные сообщения в retry-топики и DLQ.
package inbox

import "time"

// Record описывает строку таблицы inbox (DDL в migrations/inbox.sql).
type Record struct {
	// Consumer задает имя консьюмера (группы) платформы.
	Consumer string `gorm:"column:consumer;primaryKey"`
	// EventID задает UUIDv7 события.
	EventID string `gorm:"column:event_id;type:uuid;primaryKey"`
	// ProcessedAt задает время обработки события.
	ProcessedAt time.Time `gorm:"column:processed_at"`
}

// TableName задает имя таблицы inbox.
func (Record) TableName() string { return "inbox" }
