package outbox_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ZMichael3012/cbl-kit/outbox"
)

func TestNewEnvelope(t *testing.T) {
	t.Parallel()

	env, err := outbox.NewEnvelope("payment.completed", "payment.events", "pay-1", nil, []byte(`{"a":1}`))
	require.NoError(t, err)
	assert.NotEmpty(t, env.EventID)
	_, parseErr := uuid.Parse(env.EventID)
	require.NoError(t, parseErr)
	assert.Equal(t, "payment.events", env.Topic)
	assert.Equal(t, "pay-1", env.Key)
	assert.Equal(t, "payment.completed", env.Headers[outbox.HeaderEventType])
	assert.NotEmpty(t, env.Headers[outbox.HeaderOccurredAt])
	assert.Equal(t, env.EventID, env.Headers[outbox.HeaderEventID])
	require.NoError(t, env.Validate())
}

func TestNewEnvelopeKeepsProvidedEventID(t *testing.T) {
	t.Parallel()

	id := uuid.Must(uuid.NewV7()).String()
	env, err := outbox.NewEnvelope("payment.completed", "payment.events", "pay-1",
		map[string]string{outbox.HeaderEventID: id}, []byte("{}"))
	require.NoError(t, err)
	assert.Equal(t, id, env.EventID)
}

func TestNewEnvelopeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		topic   string
		key     string
		headers map[string]string
		payload []byte
	}{
		{name: "empty topic", topic: "", key: "k", payload: []byte("{}")},
		{name: "empty key", topic: "t", key: "", payload: []byte("{}")},
		{name: "empty payload", topic: "t", key: "k"},
		{name: "bad event id", topic: "t", key: "k", headers: map[string]string{outbox.HeaderEventID: "not-uuid"}, payload: []byte("{}")},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := outbox.NewEnvelope("payment.completed", tt.topic, tt.key, tt.headers, tt.payload)
			require.ErrorIs(t, err, outbox.ErrInvalidEnvelope)
		})
	}
}

func TestEnvelopeValidate(t *testing.T) {
	t.Parallel()

	env, err := outbox.NewEnvelope("payment.completed", "t", "k", nil, []byte("{}"))
	require.NoError(t, err)

	broken := env
	broken.Topic = " "
	require.ErrorIs(t, broken.Validate(), outbox.ErrInvalidEnvelope)

	broken = env
	broken.Key = " "
	require.ErrorIs(t, broken.Validate(), outbox.ErrInvalidEnvelope)

	broken = env
	broken.Payload = nil
	require.ErrorIs(t, broken.Validate(), outbox.ErrInvalidEnvelope)

	broken = env
	broken.Headers = map[string]string{outbox.HeaderEventType: " "}
	require.ErrorIs(t, broken.Validate(), outbox.ErrInvalidEnvelope)
}

func TestJSONHeadersValueScan(t *testing.T) {
	t.Parallel()

	original := outbox.JSONHeaders{"event_type": "payment.completed", "producer": "cbl-payment"}
	value, err := original.Value()
	require.NoError(t, err)

	var scanned outbox.JSONHeaders
	require.NoError(t, scanned.Scan(value))
	assert.Equal(t, original, scanned)

	require.NoError(t, scanned.Scan([]byte(`{"event_type":"x"}`)))
	assert.Equal(t, "x", scanned["event_type"])

	require.NoError(t, scanned.Scan("{\"event_type\":\"y\"}"))
	assert.Equal(t, "y", scanned["event_type"])

	require.NoError(t, scanned.Scan(nil))
	assert.Nil(t, scanned)

	require.Error(t, scanned.Scan(42))
	require.Error(t, scanned.Scan([]byte("{bad")))

	var empty outbox.JSONHeaders
	value, err = empty.Value()
	require.NoError(t, err)
	assert.Equal(t, []byte("{}"), value)
}
