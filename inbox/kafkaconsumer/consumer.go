// Пакет kafkaconsumer читает Kafka-сообщения через franz-go и передает их
// в inbox.Processor; офсеты коммитятся только после успешной обработки.
package kafkaconsumer

import (
	"context"
	"errors"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"

	"github.com/cbl/cbl-kit/inbox"
)

// Runner описывает цикл чтения Kafka с ручным коммитом офсетов.
type Runner struct {
	client    *kgo.Client
	processor *inbox.Processor
	log       *zap.Logger
}

// New создает Runner для списка топиков консьюмер-группы.
// Автокоммит отключен: коммит выполняется только после успешной обработки.
func New(brokers []string, groupID string, topics []string, processor *inbox.Processor, log *zap.Logger, opts ...kgo.Opt) (*Runner, error) {
	if processor == nil {
		return nil, errors.New("kafkaconsumer: nil processor")
	}
	if len(topics) == 0 {
		return nil, errors.New("kafkaconsumer: empty topics")
	}
	if log == nil {
		log = zap.NewNop()
	}
	base := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeTopics(topics...),
		kgo.DisableAutoCommit(),
	}
	client, err := kgo.NewClient(append(base, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("kafkaconsumer: create client: %w", err)
	}
	return &Runner{client: client, processor: processor, log: log}, nil
}

// Run читает и обрабатывает сообщения до отмены контекста.
// При ошибке обработки офсеты батча не коммитятся, и сообщения доставляются
// повторно; идемпотентность обеспечивает inbox.
func (r *Runner) Run(ctx context.Context) error {
	for {
		fetches := r.client.PollFetches(ctx)
		if fetches.IsClientClosed() {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		fetches.EachError(func(topic string, partition int32, err error) {
			r.log.Error("kafka fetch error",
				zap.String("topic", topic),
				zap.Int32("partition", partition),
				zap.Error(err),
			)
		})

		var processed []*kgo.Record
		failed := false
		fetches.EachRecord(func(record *kgo.Record) {
			if failed {
				return
			}
			msg := inbox.Message{
				Topic:   record.Topic,
				Key:     string(record.Key),
				Headers: headersFromRecord(record),
				Value:   record.Value,
			}
			if err := r.processor.Process(ctx, msg); err != nil {
				failed = true
				r.log.Error("inbox processing failed; offsets not committed",
					zap.String("topic", record.Topic),
					zap.Int32("partition", record.Partition),
					zap.Error(err),
				)
				return
			}
			processed = append(processed, record)
		})
		if failed || len(processed) == 0 {
			continue
		}
		if err := r.client.CommitRecords(ctx, processed...); err != nil {
			r.log.Error("kafka commit failed", zap.Error(err))
		}
	}
}

// Close закрывает клиент.
func (r *Runner) Close() { r.client.Close() }

// headersFromRecord переводит заголовки Kafka в карту конверта.
func headersFromRecord(record *kgo.Record) map[string]string {
	headers := make(map[string]string, len(record.Headers))
	for _, h := range record.Headers {
		headers[h.Key] = string(h.Value)
	}
	return headers
}
