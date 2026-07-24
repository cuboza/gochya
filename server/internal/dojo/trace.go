package dojo

import "context"

type traceIDContextKey struct{}

func contextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDContextKey{}, traceID)
}

// TraceIDFromContext exposes the Dojo flow identifier to instrumented
// attestation, Core and Store implementations without coupling them to HTTP.
func TraceIDFromContext(ctx context.Context) (string, bool) {
	traceID, ok := ctx.Value(traceIDContextKey{}).(string)
	return traceID, ok && traceID != ""
}
