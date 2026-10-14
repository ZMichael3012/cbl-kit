package inbox

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/cbl/cbl-kit/outbox"
)

// Message описывает входящее сообщение консьюмера.
type Message struct {
	// Topic задает топик доставки.
	Topic string
	// Key задает ключ сообщения.
	Key string
	// Headers задает заголовки конверта.
	Headers map[string]string
	// Value задает тело события.
	Value []byte
}

// Handler обрабатывает событие в транзакции, предоставленной Guard.
type Handler interface {
	// Handle выполняет бизнес-эффект идемпотентно в переданной транзакции.
	Handle(ctx context.Context, tx *gorm.DB, msg Message) error
}

// Processor связывает Guard, Handler и маршрутизацию повторной доставки.
type Processor struct {
	consumer string
	db       *gorm.DB
	guard    *Guard
	handler  Handler
	pub      outbox.Publisher
	router   *Router
	log      *zap.Logger
}

// NewProcessor создает Processor; nil-зависимости guard, router и log заменяются значениями по умолчанию.
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

// Process обрабатывает сообщение: дубликаты пропускаются, при ошибке сообщение
// уходит в retry-топик или DLQ. Возврат nil означает подтверждение исходного
// сообщения; возврат ошибки оставляет исходное сообщение на повторную доставку.
func (p *Processor) Process(ctx context.Context, msg Message) error {
	if p.db == nil || p.handler == nil {
		return errors.New("inbox: processor is not configured")
	}
	eventID := strings.TrimSpace(msg.Headers[outbox.HeaderEventID])
	if eventID == "" {
		// Сообщение без event_id нельзя ни дедуплицировать, ни ретраить:
		// оно отправляется сразу в DLQ, иначе партиция встанет в бесконечную редоставку.
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
		return errors.Join(err, envErr)
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

// sendToDLQ публикует необрабатываемое сообщение в DLQ и подтверждает исходное.
// Публикация невозможна без publisher: тогда ошибка возвращается, чтобы сообщение
// не потерялось молча.
func (p *Processor) sendToDLQ(ctx context.Context, msg Message, cause error) error {
	if p.pub == nil {
		return cause
	}
	headers := make(map[string]string, len(msg.Headers)+2)
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

// retryEnvelope формирует конверт повторной доставки с сохранением event_id.
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
