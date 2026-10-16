// Package inbox implements idempotent processing of Kafka events:
// the Guard filters out duplicates by the (consumer, event_id) pair, the Processor
// routes failed messages to retry topics and the DLQ.
package inbox

import "time"

// Record describes a row of the inbox table (DDL in migrations/inbox.sql).
type Record struct {
	// Consumer sets the platform consumer (group) name.
	Consumer string `gorm:"column:consumer;primaryKey"`
	// EventID sets the event UUIDv7.
	EventID string `gorm:"column:event_id;type:uuid;primaryKey"`
	// ProcessedAt sets the event processing time.
	ProcessedAt time.Time `gorm:"column:processed_at"`
}

// TableName sets the inbox table name.
func (Record) TableName() string { return "inbox" }
