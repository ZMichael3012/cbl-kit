// Package outbox implements the transactional outbox and the event publication relay.
// An event is written in the same transaction as the business data, and the relay
// publishes it to Kafka with at-least-once delivery.
package outbox

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidEnvelope reports an invalid event envelope.
var ErrInvalidEnvelope = errors.New("outbox: invalid envelope")

// Names of the service event headers (the CBL envelope).
const (
	// HeaderEventID sets the event UUIDv7, the inbox idempotency key.
	HeaderEventID = "event_id"
	// HeaderEventType sets the event type, for example payment.completed.
	HeaderEventType = "event_type"
	// HeaderOccurredAt sets the event time in RFC 3339 UTC.
	HeaderOccurredAt = "occurred_at"
	// HeaderProducer sets the producing service of the event.
	HeaderProducer = "producer"
	// HeaderTraceparent sets the W3C trace context.
	HeaderTraceparent = "traceparent"
	// HeaderSchemaVersion sets the event schema version.
	HeaderSchemaVersion = "schema_version"
)

// Envelope describes an outbox event ready for publication.
type Envelope struct {
	// EventID sets the event UUIDv7.
	EventID string
	// Topic sets the Kafka topic.
	Topic string
	// Key sets the partition key.
	Key string
	// Headers sets the event headers.
	Headers map[string]string
	// Payload sets the serialized event body.
	Payload []byte
}

// NewEnvelope creates an event envelope; event_id is generated as a UUIDv7 when not set.
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

// Validate checks the required envelope fields.
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
