package kafkapub_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ZMichael3012/cbl-kit/outbox"
	"github.com/ZMichael3012/cbl-kit/outbox/kafkapub"
)

func TestNewAndClose(t *testing.T) {
	t.Parallel()

	publisher, err := kafkapub.New([]string{"localhost:1"})
	require.NoError(t, err)
	publisher.Close()
}

func TestPublishCanceledContext(t *testing.T) {
	t.Parallel()

	publisher, err := kafkapub.New([]string{"localhost:1"})
	require.NoError(t, err)
	defer publisher.Close()

	env, err := outbox.NewEnvelope("payment.completed", "payment.events", "pay-1", nil, []byte("{}"))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.Error(t, publisher.Publish(ctx, env))
}
