package grpcmw

import (
	"context"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Имена элементов metadata, используемых платформой.
const (
	// MetadataMerchantID задает идентификатор мерчанта, проверенный на mTLS-периметре.
	MetadataMerchantID = "x-merchant-id"
	// MetadataRequestID задает идентификатор запроса.
	MetadataRequestID = "request_id"
	// MetadataHTTPRequestID задает заголовок идентификатора из HTTP-периметра.
	MetadataHTTPRequestID = "x-request-id"
)

type requestIDKey struct{}

// RequestID возвращает идентификатор запроса из контекста или входящей metadata.
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return incomingRequestID(ctx)
}

// MerchantID возвращает x-merchant-id из входящей metadata.
func MerchantID(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", E(KindInvalidArgument, "missing_metadata", "metadata отсутствует", nil)
	}
	for _, value := range md.Get(MetadataMerchantID) {
		if v := strings.TrimSpace(value); v != "" {
			return v, nil
		}
	}
	return "", E(KindInvalidArgument, "missing_merchant_id", "x-merchant-id не задан", nil)
}

// incomingRequestID извлекает идентификатор запроса из входящей metadata.
func incomingRequestID(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	for _, key := range []string{MetadataRequestID, MetadataHTTPRequestID} {
		for _, value := range md.Get(key) {
			if v := strings.TrimSpace(value); v != "" {
				return v
			}
		}
	}
	return ""
}

// UnaryServer объединяет recovery, request_id, логирование и маппинг ошибок.
func UnaryServer(log *zap.Logger) grpc.UnaryServerInterceptor {
	if log == nil {
		log = zap.NewNop()
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		start := time.Now()
		id := incomingRequestID(ctx)
		if id == "" {
			if v7, genErr := uuid.NewV7(); genErr == nil {
				id = v7.String()
			}
		}
		ctx = context.WithValue(ctx, requestIDKey{}, id)

		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered",
					zap.Any("panic", r),
					zap.String("stack", string(debug.Stack())),
					zap.String("method", info.FullMethod),
					zap.String("request_id", id),
				)
				err = status.Error(codes.Internal, "internal error")
			}
		}()

		resp, err = handler(ctx, req)
		if err != nil {
			err = MapError(err)
		}
		_ = grpc.SetHeader(ctx, metadata.Pairs(MetadataRequestID, id))
		logCall(log, info.FullMethod, id, time.Since(start), err)
		return resp, err
	}
}

// UnaryClient пробрасывает request_id в исходящую metadata и логирует вызовы.
func UnaryClient(log *zap.Logger) grpc.UnaryClientInterceptor {
	if log == nil {
		log = zap.NewNop()
	}
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		if id := RequestID(ctx); id != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, MetadataRequestID, id)
		}
		err := invoker(ctx, method, req, reply, cc, opts...)
		fields := []zap.Field{zap.String("method", method), zap.Duration("duration", time.Since(start))}
		if err != nil {
			log.Warn("grpc call failed", append(fields, zap.Error(err))...)
		} else {
			log.Debug("grpc call", fields...)
		}
		return err
	}
}

// StreamServer объединяет recovery и маппинг ошибок потоковых вызовов.
func StreamServer(log *zap.Logger) grpc.StreamServerInterceptor {
	if log == nil {
		log = zap.NewNop()
	}
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered",
					zap.Any("panic", r),
					zap.String("stack", string(debug.Stack())),
					zap.String("method", info.FullMethod),
				)
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		err = handler(srv, ss)
		if err != nil {
			err = MapError(err)
		}
		return err
	}
}

// StreamClient пробрасывает request_id и логирует открытие потоковых вызовов.
func StreamClient(log *zap.Logger) grpc.StreamClientInterceptor {
	if log == nil {
		log = zap.NewNop()
	}
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		start := time.Now()
		if id := RequestID(ctx); id != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, MetadataRequestID, id)
		}
		cs, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			log.Warn("grpc stream failed to open", zap.String("method", method), zap.Duration("duration", time.Since(start)), zap.Error(err))
		}
		return cs, err
	}
}

// logCall пишет структурированную запись о вызове с учетом кода статуса.
func logCall(log *zap.Logger, method, requestID string, dur time.Duration, err error) {
	fields := []zap.Field{
		zap.String("method", method),
		zap.String("request_id", requestID),
		zap.Duration("duration", dur),
	}
	switch status.Code(err) {
	case codes.OK:
		log.Info("grpc request", fields...)
	case codes.Internal, codes.Unavailable, codes.Unknown:
		log.Error("grpc request failed", append(fields, zap.Error(err))...)
	default:
		log.Warn("grpc request rejected", append(fields, zap.Error(err))...)
	}
}
