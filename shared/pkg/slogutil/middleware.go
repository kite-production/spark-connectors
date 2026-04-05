package slogutil

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// HTTPMiddleware returns an HTTP middleware that logs every request with
// structured fields: method, path, status, duration, remote_addr.
//
// Sensitive headers (Authorization, Cookie) are never logged.
// The trace_id and span_id are automatically injected by the traceHandler
// from the request context (if an upstream OTel middleware created a span).
func HTTPMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(sw, r)

			duration := time.Since(start)
			level := slog.LevelInfo
			if sw.status >= 500 {
				level = slog.LevelError
			} else if sw.status >= 400 {
				level = slog.LevelWarn
			}

			logger.LogAttrs(r.Context(), level, "http request",
				slog.String("method", r.Method),
				slog.String("path", sanitizePath(r.URL.Path)),
				slog.Int("status", sw.status),
				slog.Duration("duration", duration),
				slog.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}

// statusWriter captures the HTTP status code written by the handler.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}

// sanitizePath removes query parameters and truncates long paths to prevent
// log injection or excessive log volume from pathological URLs.
func sanitizePath(path string) string {
	// Strip query string if somehow present
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	if len(path) > 256 {
		path = path[:256] + "…"
	}
	return path
}
