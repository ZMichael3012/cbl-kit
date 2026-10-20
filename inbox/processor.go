package inbox

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ZMichael3012/cbl-kit/outbox"
)

// Message describes an incoming consumer message.
type Message struct {
	// Topic sets the delivery topic.
	Topic string
	// Key sets the message key.
	Key string
	// Headers sets the envelope headers.
	Headers map[string]string
	// Value sets the event body.
	Value []byte
}

// Handler processes an event in the transaction provided by the Guard.
type Handler interface {
	// Handle performs the business effect idempotently in the given transaction.
	Handle(ctx context.Context, tx *gorm.DB, msg Message) error
}

// Processor ties together the Guard, the Handler and retry routing.
type Processor struct {
	consumer string
	db       *gorm.DB
	guard    *Guard
	handler  Handler
	pub      outbox.Publisher
	router   *Router
	log      *zap.Logger
}

// NewProcessor creates a Processor; nil guard, router and log dependencies are replaced with defaults.
func NewProcessor(consumer string, db *gorm.DB, guard *Guard, handler Handler, pub outbox.Publisher, router *Router, log *zap.Logger) *Processor {
	if guard == nil {
		guard = NewGuard()
	}
	if router == nil {
		router = NewRouter("", 0)
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &Processor{
		consumer: consumer,
		db:       db,
		guard:    guard,
		handler:  handler,
		pub:      pub,
		router:   router,
		log:      log,
	}
}

// Process handles a message: duplicates are skipped, and on error the message
// goes to a retry topic or the DLQ. Returning nil acknowledges the original
// message; returning an error leaves the original message for redelivery.
func (p *Processor) Process(ctx context.Context, msg Message) error {
	if p.db == nil || p.handler == nil {
		return errors.New("inbox: processor is not configured")
	}
	eventID := strings.TrimSpace(msg.Headers[outbox.HeaderEventID])
	if eventID == "" {
		// A message without event_id can be neither deduplicated nor retried:
		// it goes straight to the DLQ, otherwise the partition would loop on redelivery.
		return p.sendToDLQ(ctx, msg, fmt.Errorf("%w: %s отсутствует", ErrInvalidMessage, outbox.HeaderEventID))
	}
	processed, err := p.guard.Process(ctx, p.db, p.consumer, eventID, func(tx *gorm.DB) error {
		return p.handler.Handle(ctx, tx, msg)
	})
	if err == nil {
		if !processed {
			p.log.Debug("duplicate event skipped", zap.String("event_id", eventID), zap.String("topic", msg.Topic))
		}
		return nil
	}
	if p.pub == nil {
		return fmt.Errorf("inbox: publish retry: publisher is nil: %w", err)
	}
	attempt := ParseAttempt(msg.Headers[HeaderRetryAttempt]) + 1
	destination, isDLQ := p.router.Next(msg.Topic, attempt)
	env, envErr := p.retryEnvelope(msg, destination, attempt, err)
	if envErr != nil {
		// The retry envelope cannot be built (broken event_id or missing event_type):
		// redelivering such a message would loop the partition, so it goes to the
		// DLQ with sanitized service fields.
		return p.sendToDLQ(ctx, msg, errors.Join(err, envErr))
	}
	if pubErr := p.pub.Publish(ctx, env); pubErr != nil {
		return errors.Join(err, fmt.Errorf("inbox: publish retry: %w", pubErr))
	}
	if isDLQ {
		p.log.Error("event moved to dlq",
			zap.String("event_id", eventID),
			zap.String("topic", destination),
			zap.Error(err),
		)
	} else {
		p.log.Warn("event scheduled for retry",
			zap.String("event_id", eventID),
			zap.String("topic", destination),
			zap.Int("attempt", attempt),
			zap.Error(err),
		)
	}
	return nil
}

// sendToDLQ publishes an unprocessable message to the DLQ and acknowledges the original.
// Publication is impossible without a publisher: then the error is returned so that the
// message is not lost silently. An invalid event_id is replaced with a fresh UUIDv7 (the
// original value is preserved in x-original-event-id), a missing event_type becomes "unknown",
// otherwise the DLQ envelope would not pass validation and the message would loop the partition.
func (p *Processor) sendToDLQ(ctx context.Context, msg Message, cause error) error {
	if p.pub == nil {
		return cause
	}
	headers := make(map[string]string, len(msg.Headers)+3)
	for k, v := range msg.Headers {
		headers[k] = v
	}
	headers[HeaderError] = cause.Error()
	eventType := strings.TrimSpace(headers[outbox.HeaderEventType])
	if eventType == "" {
		eventType = "unknown"
	}
	key := strings.TrimSpace(msg.Key)
	if key == "" {
		key = "unknown"
	}
	payload := msg.Value
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	if _, err := uuid.Parse(strings.TrimSpace(headers[outbox.HeaderEventID])); err != nil {
		if original := strings.TrimSpace(headers[outbox.HeaderEventID]); original != "" {
			headers[HeaderOriginalEventID] = original
		}
		delete(headers, outbox.HeaderEventID)
	}
	destination := MainTopicFrom(msg.Topic) + DLQSuffix
	env, err := outbox.NewEnvelope(eventType, destination, key, headers, payload)
	if err != nil {
		return errors.Join(cause, err)
	}
	if err := p.pub.Publish(ctx, env); err != nil {
		return errors.Join(cause, fmt.Errorf("inbox: publish dlq: %w", err))
	}
	p.log.Error("unprocessable event moved to dlq", zap.String("topic", destination), zap.Error(cause))
	return nil
}

// retryEnvelope builds a redelivery envelope while preserving the event_id.
func (p *Processor) retryEnvelope(msg Message, destination string, attempt int, cause error) (outbox.Envelope, error) {
	headers := make(map[string]string, len(msg.Headers)+2)
	for k, v := range msg.Headers {
		headers[k] = v
	}
	headers[HeaderRetryAttempt] = strconv.Itoa(attempt)
	headers[HeaderError] = cause.Error()
	eventType := strings.TrimSpace(headers[outbox.HeaderEventType])
	if eventType == "" {
		return outbox.Envelope{}, fmt.Errorf("%w: %s отсутствует", ErrInvalidMessage, outbox.HeaderEventType)
	}
	return outbox.NewEnvelope(eventType, destination, msg.Key, headers, msg.Value)
}
