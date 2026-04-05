package slogutil

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// UnaryServerInterceptor returns a gRPC unary server interceptor that logs
// every RPC call with structured fields: method, code, duration.
//
// The trace_id and span_id are automatically injected by the traceHandler
// from the request context (OTel gRPC interceptors should run before this one
// to ensure span context is available).
func UnaryServerInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start)

		code := status.Code(err)
		level := slog.LevelInfo
		if err != nil {
			level = slog.LevelError
		}

		logger.LogAttrs(ctx, level, "grpc unary",
			slog.String("method", info.FullMethod),
			slog.String("code", code.String()),
			slog.Duration("duration", duration),
		)

		return resp, err
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor that logs
// the start and end of every streaming RPC.
func StreamServerInterceptor(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		err := handler(srv, ss)
		duration := time.Since(start)

		code := status.Code(err)
		level := slog.LevelInfo
		if err != nil {
			level = slog.LevelError
		}

		logger.LogAttrs(ss.Context(), level, "grpc stream",
			slog.String("method", info.FullMethod),
			slog.String("code", code.String()),
			slog.Duration("duration", duration),
		)

		return err
	}
}
