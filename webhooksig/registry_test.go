package webhooksig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cbl/cbl-kit/webhooksig"
)

func TestRegistry(t *testing.T) {
	t.Parallel()
	registry := webhooksig.NewRegistry()
	strategy := webhooksig.TokenEcho{}

	require.NoError(t, registry.Register("coingate", strategy))

	got, ok := registry.Lookup("coingate")
	require.True(t, ok)
	require.Equal(t, strategy, got)

	_, ok = registry.Lookup("coingate ")
	require.True(t, ok)

	_, ok = registry.Lookup("unknown")
	require.False(t, ok)

	require.ErrorIs(t, registry.Register("coingate", strategy), webhooksig.ErrMalformed)
	require.ErrorIs(t, registry.Register("", strategy), webhooksig.ErrMalformed)
	require.ErrorIs(t, registry.Register("nil", nil), webhooksig.ErrMalformed)

	var typedNil *webhooksig.TokenEcho
	require.ErrorIs(t, registry.Register("typed-nil", typedNil), webhooksig.ErrMalformed)
}
