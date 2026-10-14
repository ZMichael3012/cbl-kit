// Пакет outbox реализует транзакционный outbox и relay публикации событий.
// Событие пишется в ту же транзакцию, что и бизнес-данные, а relay публикует
// его в Kafka по модели at-least-once.
package outbox

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidEnvelope сообщает о некорректном конверте события.
var ErrInvalidEnvelope = errors.New("outbox: invalid envelope")

// Имена служебных заголовков события (конверт CBL).
const (
	// HeaderEventID задает UUIDv7 события, ключ идемпотентности inbox.
	HeaderEventID = "event_id"
	// HeaderEventType задает тип события, например payment.completed.
	HeaderEventType = "event_type"
	// HeaderOccurredAt задает время события в RFC 3339 UTC.
	HeaderOccurredAt = "occurred_at"
	// HeaderProducer задает сервис-продюсер события.
	HeaderProducer = "producer"
	// HeaderTraceparent задает W3C trace context.
	HeaderTraceparent = "traceparent"
	// HeaderSchemaVersion задает версию схемы события.
	HeaderSchemaVersion = "schema_version"
)

// Envelope описывает событие outbox, готовое к публикации.
type Envelope struct {
	// EventID задает UUIDv7 события.
	EventID string
	// Topic задает топик Kafka.
	Topic string
	// Key задает ключ партиции.
	Key string
	// Headers задает заголовки события.
	Headers map[string]string
	// Payload задает сериализованное тело события.
	Payload []byte
}

// NewEnvelope создает конверт события; event_id генерируется как UUIDv7, если не задан.
func NewEnvelope(eventType, topic, key string, headers map[string]string, payload []byte) (Envelope, error) {
	merged := make(map[string]string, len(headers)+4)
	for k, v := range headers {
		merged[k] = v
	}
	id := strings.TrimSpace(merged[HeaderEventID])
	if id == "" {
		v7, err := uuid.NewV7()
		if err != nil {
			return Envelope{}, fmt.Errorf("outbox: generate event_id: %w", err)
		}
		id = v7.String()
	}
	merged[HeaderEventID] = id
	merged[HeaderEventType] = eventType
	if strings.TrimSpace(merged[HeaderOccurredAt]) == "" {
		merged[HeaderOccurredAt] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	env := Envelope{EventID: id, Topic: topic, Key: key, Headers: merged, Payload: payload}
	if err := env.Validate(); err != nil {
		return Envelope{}, err
	}
	return env, nil
}

// Validate проверяет обязательные поля конверта.
func (e Envelope) Validate() error {
	if _, err := uuid.Parse(e.EventID); err != nil {
		return fmt.Errorf("%w: event_id %q", ErrInvalidEnvelope, e.EventID)
	}
	if strings.TrimSpace(e.Topic) == "" {
		return fmt.Errorf("%w: empty topic", ErrInvalidEnvelope)
	}
	if strings.TrimSpace(e.Key) == "" {
		return fmt.Errorf("%w: empty key", ErrInvalidEnvelope)
	}
	if strings.TrimSpace(e.Headers[HeaderEventType]) == "" {
		return fmt.Errorf("%w: empty %s", ErrInvalidEnvelope, HeaderEventType)
	}
	if len(e.Payload) == 0 {
		return fmt.Errorf("%w: empty payload", ErrInvalidEnvelope)
	}
	return nil
}
