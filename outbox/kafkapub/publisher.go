// Пакет kafkapub публикует конверты outbox в Kafka через franz-go.
package kafkapub

import (
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/cbl/cbl-kit/outbox"
)

// Publisher публикует события в Kafka идемпотентным продюсером.
type Publisher struct {
	client *kgo.Client
}

// New создает Publisher с идемпотентным продюсером и acks=all.
func New(brokers []string, opts ...kgo.Opt) (*Publisher, error) {
	// RecordDeliveryTimeout ограничивает время публикации: relay вызывает Publish
	// внутри транзакции БД, и бесконечное ожидание несуществующего топика
	// недопустимо удерживает блокировки (T-KK5).
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

// Publish отправляет конверт в топик с заголовками события.
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

// Close закрывает клиент, дожидаясь доставки буферизованных записей.
func (p *Publisher) Close() {
	p.client.Close()
}
