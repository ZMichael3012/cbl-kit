package storagex_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cbl/cbl-kit/storagex"
)

func TestMigrateMissingDir(t *testing.T) {
	t.Parallel()

	err := storagex.Migrate(context.Background(), nil, "no-such-dir")
	require.Error(t, err)
}
