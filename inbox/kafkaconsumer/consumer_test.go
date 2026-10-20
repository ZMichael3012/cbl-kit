package kafkaconsumer_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ZMichael3012/cbl-kit/inbox"
	"github.com/ZMichael3012/cbl-kit/inbox/kafkaconsumer"
)

func TestNewValidation(t *testing.T) {
	t.Parallel()

	_, err := kafkaconsumer.New([]string{"localhost:1"}, "group", []string{"topic"}, nil, nil)
	require.Error(t, err)

	processor := inbox.NewProcessor("consumer", nil, nil, nil, nil, nil, nil)
	_, err = kafkaconsumer.New([]string{"localhost:1"}, "group", nil, processor, nil)
	require.Error(t, err)

	runner, err := kafkaconsumer.New([]string{"localhost:1"}, "group", []string{"topic"}, processor, nil)
	require.NoError(t, err)
	runner.Close()
}
