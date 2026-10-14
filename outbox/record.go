package outbox

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// Record описывает строку таблицы outbox (DDL в migrations/outbox.sql).
type Record struct {
	// ID задает монотонный идентификатор строки.
	ID int64 `gorm:"column:id;primaryKey"`
	// EventID задает UUIDv7 события.
	EventID string `gorm:"column:event_id;type:uuid;uniqueIndex"`
	// Topic задает топик Kafka.
	Topic string `gorm:"column:topic"`
	// Key задает ключ партиции.
	Key string `gorm:"column:key"`
	// Headers задает заголовки события в формате jsonb.
	Headers JSONHeaders `gorm:"column:headers;type:jsonb"`
	// Payload задает тело события.
	Payload []byte `gorm:"column:payload"`
	// CreatedAt задает время создания записи.
	CreatedAt time.Time `gorm:"column:created_at"`
	// PublishedAt задает время публикации; nil означает ожидание публикации.
	PublishedAt *time.Time `gorm:"column:published_at"`
}

// TableName задает имя таблицы outbox.
func (Record) TableName() string { return "outbox" }

// JSONHeaders сериализует заголовки события в jsonb.
type JSONHeaders map[string]string

// Value реализует driver.Valuer для jsonb.
func (h JSONHeaders) Value() (driver.Value, error) {
	if h == nil {
		return []byte("{}"), nil
	}
	raw, err := json.Marshal(map[string]string(h))
	if err != nil {
		return nil, fmt.Errorf("outbox: encode headers: %w", err)
	}
	return raw, nil
}

// Scan реализует sql.Scanner для jsonb.
func (h *JSONHeaders) Scan(value any) error {
	if value == nil {
		*h = nil
		return nil
	}
	var raw []byte
	switch v := value.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("outbox: unsupported headers type %T", value)
	}
	parsed := map[string]string{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("outbox: decode headers: %w", err)
	}
	*h = parsed
	return nil
}
