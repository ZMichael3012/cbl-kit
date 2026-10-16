package inbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrInvalidMessage reports an invalid incoming message.
var ErrInvalidMessage = errors.New("inbox: invalid message")

// Guard ensures that each consumer processes an event only once.
type Guard struct{}

// NewGuard creates a Guard.
func NewGuard() *Guard { return &Guard{} }

// Process runs fn in the same transaction as the inbox record insert.
// If the event has already been processed, fn is not called and processed=false.
// An fn error rolls back the whole transaction, including the inbox record, so
// a redelivery calls the handler again.
func (g *Guard) Process(ctx context.Context, db *gorm.DB, consumer, eventID string, fn func(tx *gorm.DB) error) (bool, error) {
	if db == nil {
		return false, errors.New("inbox: nil db")
	}
	if strings.TrimSpace(consumer) == "" {
		return false, fmt.Errorf("%w: empty consumer", ErrInvalidMessage)
	}
	if _, err := uuid.Parse(eventID); err != nil {
		return false, fmt.Errorf("%w: event_id %q", ErrInvalidMessage, eventID)
	}
	if fn == nil {
		return false, errors.New("inbox: nil handler")
	}
	processed := false
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Record{
			Consumer:    consumer,
			EventID:     eventID,
			ProcessedAt: time.Now().UTC(),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		if err := fn(tx); err != nil {
			return err
		}
		processed = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return processed, nil
}
