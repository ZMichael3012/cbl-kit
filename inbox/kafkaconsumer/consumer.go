// Package kafkaconsumer reads Kafka messages via franz-go and passes them
// to inbox.Processor; offsets are committed only after successful processing.
package kafkaconsumer

import (
	"context"
	"errors"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"

	"github.com/ZMichael3012/cbl-kit/inbox"
)

// Runner describes the Kafka read loop with manual offset commits.
type Runner struct {
	client    *kgo.Client
	processor *inbox.Processor
	log       *zap.Logger
}

// New creates a Runner for the consumer group's topic list.
// Auto-commit is disabled: a commit happens only after successful processing.
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

// Run reads and processes messages until the context is canceled.
// On a processing error the batch offsets are not committed and the messages
// are redelivered; the inbox ensures idempotency.
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

// Close closes the client.
func (r *Runner) Close() { r.client.Close() }

// headersFromRecord converts Kafka headers into an envelope map.
func headersFromRecord(record *kgo.Record) map[string]string {
	headers := make(map[string]string, len(record.Headers))
	for _, h := range record.Headers {
		headers[h.Key] = string(h.Value)
	}
	return headers
}
