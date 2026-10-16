package outbox

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Store writes events to the outbox within the business-change transaction.
type Store struct{}

// NewStore creates a Store.
func NewStore() *Store { return &Store{} }

// Append adds an event to the outbox in the given transaction.
// A nil transaction is forbidden: the event must be written atomically with the business data.
func (s *Store) Append(ctx context.Context, tx *gorm.DB, env Envelope) error {
	if tx == nil {
		return errors.New("outbox: nil transaction")
	}
	if err := env.Validate(); err != nil {
		return err
	}
	record := Record{
		EventID:   env.EventID,
		Topic:     env.Topic,
		Key:       env.Key,
		Headers:   JSONHeaders(env.Headers),
		Payload:   env.Payload,
		CreatedAt: time.Now().UTC(),
	}
	return tx.WithContext(ctx).Create(&record).Error
}
