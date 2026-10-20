package inbox_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ZMichael3012/cbl-kit/inbox"
)

func TestRouterNext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		current     string
		attempt     int
		maxAttempts int
		wantTopic   string
		wantDLQ     bool
	}{
		{name: "first failure", current: "payment.events", attempt: 1, maxAttempts: 3, wantTopic: "payment.events.retry.5s"},
		{name: "second failure", current: "payment.events.retry.5s", attempt: 2, maxAttempts: 3, wantTopic: "payment.events.retry.1m"},
		{name: "third failure", current: "payment.events.retry.1m", attempt: 3, maxAttempts: 3, wantTopic: "payment.events.retry.10m"},
		{name: "exhausted to dlq", current: "payment.events.retry.10m", attempt: 4, maxAttempts: 3, wantTopic: "payment.events.dlq", wantDLQ: true},
		{name: "single attempt", current: "ledger.events", attempt: 2, maxAttempts: 1, wantTopic: "ledger.events.dlq", wantDLQ: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			router := inbox.NewRouter("payment.events", tt.maxAttempts)
			topic, isDLQ := router.Next(tt.current, tt.attempt)
			assert.Equal(t, tt.wantTopic, topic)
			assert.Equal(t, tt.wantDLQ, isDLQ)
		})
	}
}

func TestMainTopicFrom(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"payment.events":           "payment.events",
		"payment.events.retry.5s":  "payment.events",
		"payment.events.retry.1m":  "payment.events",
		"payment.events.retry.10m": "payment.events",
		"payment.events.dlq":       "payment.events",
		"gateway.webhooks.raw":     "gateway.webhooks.raw",
		"gateway.events.canonical": "gateway.events.canonical",
	}
	for topic, want := range tests {
		assert.Equal(t, want, inbox.MainTopicFrom(topic))
	}
}

func TestParseAttempt(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, inbox.ParseAttempt(""))
	assert.Equal(t, 0, inbox.ParseAttempt("bad"))
	assert.Equal(t, 0, inbox.ParseAttempt("-2"))
	assert.Equal(t, 3, inbox.ParseAttempt("3"))
	assert.Equal(t, 2, inbox.ParseAttempt(" 2 "))
}

func TestProcessorNotConfigured(t *testing.T) {
	t.Parallel()

	processor := inbox.NewProcessor("c", nil, nil, nil, nil, nil, nil)
	require.Error(t, processor.Process(t.Context(), inbox.Message{}))
}
