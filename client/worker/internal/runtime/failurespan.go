// flow/client/worker/internal/runtime/failurespan.go
package runtime

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// tracer 每次调用获取当前全局 provider 的 tracer。
func tracer() oteltrace.Tracer { return otel.Tracer(tracerName) }

// recordFailureSpan 为高频 RPC（poll/heartbeat）的失败创建错误 span。
// 关联 ctx 中的父 span；成功路径不产生 span（dial 层已用 filter 排除），避免 trace 量过大。
func recordFailureSpan(ctx context.Context, name string, workerID int64, err error) {
	_, span := tracer().Start(ctx, name,
		oteltrace.WithAttributes(attribute.Int64(attrWorkerID, workerID)),
	)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.End()
}
