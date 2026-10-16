// Package kafkapub publishes outbox envelopes to Kafka via franz-go.
package kafkapub

import (
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/cbl/cbl-kit/outbox"
)

// Publisher publishes events to Kafka with an idempotent producer.
type Publisher struct {
	client *kgo.Client
}

// New creates a Publisher with an idempotent producer and acks=all.
func New(brokers []string, opts ...kgo.Opt) (*Publisher, error) {
	// RecordDeliveryTimeout limits the publication time: the relay calls Publish
	// inside a DB transaction, and an endless wait for a non-existent topic
	// unacceptably holds row locks (T-KK5).
	base := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()),
		kgo.RecordDeliveryTimeout(30 * time.Second),
	}
	client, err := kgo.NewClient(append(base, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("kafkapub: create client: %w", err)
	}
	return &Publisher{client: client}, nil
}

// Publish sends the envelope to the topic with the event headers.
func (p *Publisher) Publish(ctx context.Context, env outbox.Envelope) error {
	record := &kgo.Record{
		Topic: env.Topic,
		Key:   []byte(env.Key),
		Value: env.Payload,
	}
	for k, v := range env.Headers {
		record.Headers = append(record.Headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
	}
	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("kafkapub: publish %s: %w", env.EventID, err)
	}
	return nil
}

// Close closes the client, waiting for buffered records to be delivered.
func (p *Publisher) Close() {
	p.client.Close()
}
