package outbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ZMichael3012/cbl-kit/outbox"
)

func TestRelayNotConfigured(t *testing.T) {
	t.Parallel()

	relay := outbox.NewRelay(nil, nil, nil, outbox.WithBatch(5), outbox.WithInterval(time.Second), outbox.WithMetrics(nil))

	_, err := relay.PublishBatch(context.Background())
	require.Error(t, err)

	err = relay.Run(context.Background())
	require.Error(t, err)
}
