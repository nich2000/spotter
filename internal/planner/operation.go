package planner

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

type operationKey struct{}
type triggerKey struct{}

var operationSequence atomic.Uint64

// WithOperation gives collection, model calls and audit records a shared identifier.
func WithOperation(ctx context.Context) (context.Context, string) {
	id := fmt.Sprintf("%d-%d-%d", time.Now().UnixNano(), os.Getpid(), operationSequence.Add(1))
	return context.WithValue(ctx, operationKey{}, id), id
}
func OperationID(ctx context.Context) string {
	id, _ := ctx.Value(operationKey{}).(string)
	return id
}
func WithTrigger(ctx context.Context, trigger string) context.Context {
	return context.WithValue(ctx, triggerKey{}, trigger)
}
func Trigger(ctx context.Context) string {
	trigger, _ := ctx.Value(triggerKey{}).(string)
	if trigger == "" {
		return "unspecified"
	}
	return trigger
}
