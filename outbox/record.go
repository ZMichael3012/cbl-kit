package outbox

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// Record describes an outbox table row (DDL in migrations/outbox.sql).
type Record struct {
	// ID sets the monotonic row identifier.
	ID int64 `gorm:"column:id;primaryKey"`
	// EventID sets the event UUIDv7.
	EventID string `gorm:"column:event_id;type:uuid;uniqueIndex"`
	// Topic sets the Kafka topic.
	Topic string `gorm:"column:topic"`
	// Key sets the partition key.
	Key string `gorm:"column:key"`
	// Headers sets the event headers in jsonb format.
	Headers JSONHeaders `gorm:"column:headers;type:jsonb"`
	// Payload sets the event body.
	Payload []byte `gorm:"column:payload"`
	// CreatedAt sets the row creation time.
	CreatedAt time.Time `gorm:"column:created_at"`
	// PublishedAt sets the publication time; nil means waiting for publication.
	PublishedAt *time.Time `gorm:"column:published_at"`
}

// TableName sets the outbox table name.
func (Record) TableName() string { return "outbox" }

// JSONHeaders serializes event headers to jsonb.
type JSONHeaders map[string]string

// Value implements driver.Valuer for jsonb.
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

// Scan implements sql.Scanner for jsonb.
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
