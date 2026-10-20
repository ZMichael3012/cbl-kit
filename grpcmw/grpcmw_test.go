package grpcmw_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/ZMichael3012/cbl-kit/grpcmw"
)

func TestMapError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{name: "not found", err: grpcmw.E(grpcmw.KindNotFound, "payment_not_found", "платеж не найден", nil), want: codes.NotFound},
		{name: "invalid argument", err: grpcmw.E(grpcmw.KindInvalidArgument, "bad_request", "некорректный запрос", nil), want: codes.InvalidArgument},
		{name: "already exists", err: grpcmw.E(grpcmw.KindAlreadyExists, "duplicate", "уже существует", nil), want: codes.AlreadyExists},
		{name: "failed precondition", err: grpcmw.E(grpcmw.KindFailedPrecondition, "bad_state", "недопустимое состояние", nil), want: codes.FailedPrecondition},
		{name: "resource exhausted", err: grpcmw.E(grpcmw.KindResourceExhausted, "rate_limited", "лимит превышен", nil), want: codes.ResourceExhausted},
		{name: "unavailable", err: grpcmw.E(grpcmw.KindUnavailable, "gateway_down", "шлюз недоступен", nil), want: codes.Unavailable},
		{name: "unauthenticated", err: grpcmw.E(grpcmw.KindUnauthenticated, "missing_token", "token is missing", nil), want: codes.Unauthenticated},
		{name: "permission denied", err: grpcmw.E(grpcmw.KindPermissionDenied, "no_rights", "role has no rights", nil), want: codes.PermissionDenied},
		{name: "internal", err: grpcmw.E(grpcmw.KindInternal, "db_error", "секретная деталь", nil), want: codes.Internal},
		{name: "wrapped", err: wrap(grpcmw.E(grpcmw.KindNotFound, "x", "нет", nil)), want: codes.NotFound},
		{name: "unknown", err: errors.New("boom"), want: codes.Internal},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mapped := grpcmw.MapError(tt.err)
			require.Equal(t, tt.want, status.Code(mapped))
		})
	}

	require.NoError(t, grpcmw.MapError(nil))

	existing := status.Error(codes.DeadlineExceeded, "deadline")
	require.Equal(t, existing, grpcmw.MapError(existing))

	mapped := grpcmw.MapError(grpcmw.E(grpcmw.KindInternal, "db_error", "секретная деталь", nil))
	assert.NotContains(t, mapped.Error(), "секретная деталь")
}

func wrap(err error) error { return errors.Join(errors.New("outer"), err) }

func TestKindOfAndCodeOf(t *testing.T) {
	t.Parallel()

	err := grpcmw.E(grpcmw.KindFailedPrecondition, "payment_already_detected", "платеж уже обнаружен", errors.New("state machine"))
	assert.Equal(t, grpcmw.KindFailedPrecondition, grpcmw.KindOf(err))
	assert.Equal(t, "payment_already_detected", grpcmw.CodeOf(err))
	assert.Equal(t, grpcmw.KindInternal, grpcmw.KindOf(errors.New("plain")))
	assert.Equal(t, "", grpcmw.CodeOf(errors.New("plain")))
	assert.Contains(t, err.Error(), "state machine")
}

func TestUnaryServerInterceptor(t *testing.T) {
	t.Parallel()

	interceptor := grpcmw.UnaryServer(zap.NewNop())
	info := &grpc.UnaryServerInfo{FullMethod: "/billing.test.v1.Test/Ping"}

	t.Run("success", func(t *testing.T) {
		handler := func(ctx context.Context, req any) (any, error) { return "pong", nil }
		resp, err := interceptor(context.Background(), nil, info, handler)
		require.NoError(t, err)
		assert.Equal(t, "pong", resp)
	})

	t.Run("domain error mapped", func(t *testing.T) {
		handler := func(ctx context.Context, req any) (any, error) {
			return nil, grpcmw.E(grpcmw.KindNotFound, "payment_not_found", "платеж не найден", nil)
		}
		_, err := interceptor(context.Background(), nil, info, handler)
		require.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("panic recovered", func(t *testing.T) {
		handler := func(ctx context.Context, req any) (any, error) { panic("boom") }
		_, err := interceptor(context.Background(), nil, info, handler)
		require.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("request id propagated to context", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(grpcmw.MetadataRequestID, "req-42"))
		var got string
		handler := func(ctx context.Context, req any) (any, error) {
			got = grpcmw.RequestID(ctx)
			return nil, nil
		}
		_, err := interceptor(ctx, nil, info, handler)
		require.NoError(t, err)
		assert.Equal(t, "req-42", got)
	})

	t.Run("request id generated when missing", func(t *testing.T) {
		var got string
		handler := func(ctx context.Context, req any) (any, error) {
			got = grpcmw.RequestID(ctx)
			return nil, nil
		}
		_, err := interceptor(context.Background(), nil, info, handler)
		require.NoError(t, err)
		assert.NotEmpty(t, got)
	})
}

func TestMerchantID(t *testing.T) {
	t.Parallel()

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(grpcmw.MetadataMerchantID, "merchant-1"))
	id, err := grpcmw.MerchantID(ctx)
	require.NoError(t, err)
	assert.Equal(t, "merchant-1", id)

	_, err = grpcmw.MerchantID(context.Background())
	require.Error(t, err)
	assert.Equal(t, grpcmw.KindInvalidArgument, grpcmw.KindOf(err))

	blank := metadata.NewIncomingContext(context.Background(), metadata.Pairs(grpcmw.MetadataMerchantID, "   "))
	_, err = grpcmw.MerchantID(blank)
	require.Error(t, err)
}

type fakeServerStream struct {
	grpc.ServerStream
}

func TestStreamServerInterceptor(t *testing.T) {
	t.Parallel()

	interceptor := grpcmw.StreamServer(zap.NewNop())
	info := &grpc.StreamServerInfo{FullMethod: "/billing.test.v1.Test/Watch"}

	t.Run("success", func(t *testing.T) {
		handler := func(srv any, stream grpc.ServerStream) error { return nil }
		require.NoError(t, interceptor(nil, &fakeServerStream{}, info, handler))
	})

	t.Run("mapped", func(t *testing.T) {
		handler := func(srv any, stream grpc.ServerStream) error {
			return grpcmw.E(grpcmw.KindUnavailable, "gateway_down", "шлюз недоступен", nil)
		}
		err := interceptor(nil, &fakeServerStream{}, info, handler)
		require.Equal(t, codes.Unavailable, status.Code(err))
	})

	t.Run("panic recovered", func(t *testing.T) {
		handler := func(srv any, stream grpc.ServerStream) error { panic("boom") }
		err := interceptor(nil, &fakeServerStream{}, info, handler)
		require.Equal(t, codes.Internal, status.Code(err))
	})
}

func TestStreamClientInterceptor(t *testing.T) {
	t.Parallel()

	interceptor := grpcmw.StreamClient(zap.NewNop())
	desc := &grpc.StreamDesc{StreamName: "Watch"}

	t.Run("success", func(t *testing.T) {
		streamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			return nil, nil
		}
		stream, err := interceptor(context.Background(), desc, nil, "/m", streamer)
		require.NoError(t, err)
		require.Nil(t, stream)
	})

	t.Run("error", func(t *testing.T) {
		streamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			return nil, errors.New("dial failed")
		}
		_, err := interceptor(context.Background(), desc, nil, "/m", streamer)
		require.Error(t, err)
	})
}

func TestUnaryClientPropagatesRequestID(t *testing.T) {
	t.Parallel()

	interceptor := grpcmw.UnaryClient(zap.NewNop())
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(grpcmw.MetadataRequestID, "req-9"))

	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		require.True(t, ok)
		require.Equal(t, []string{"req-9"}, md.Get(grpcmw.MetadataRequestID))
		return nil
	}

	require.NoError(t, interceptor(ctx, "/billing.test.v1.Test/Ping", nil, nil, nil, invoker))
}
