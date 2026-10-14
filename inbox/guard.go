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

// ErrInvalidMessage сообщает о некорректном входящем сообщении.
var ErrInvalidMessage = errors.New("inbox: invalid message")

// Guard обеспечивает однократную обработку события каждым консьюмером.
type Guard struct{}

// NewGuard создает Guard.
func NewGuard() *Guard { return &Guard{} }

// Process выполняет fn в одной транзакции со вставкой inbox-записи.
// Если событие уже обработано, fn не вызывается и processed=false.
// Ошибка fn откатывает всю транзакцию, включая inbox-запись, поэтому
// повторная доставка снова вызовет обработчик.
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
