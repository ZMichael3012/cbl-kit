package outbox

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Store записывает события в outbox в транзакции бизнес-изменения.
type Store struct{}

// NewStore создает Store.
func NewStore() *Store { return &Store{} }

// Append добавляет событие в outbox в переданной транзакции.
// nil-транзакция запрещена: событие обязано быть записано атомарно с бизнес-данными.
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
