package runtime

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// tracer 每次调用都从当前全局 provider 取 tracer。
func tracer() oteltrace.Tracer { return otel.Tracer(tracerName) }

// recordFailureSpan 给高频 RPC（poll/heartbeat）的失败建一个错误 span，挂在 ctx 里的父 span 下。
// 成功路径不产生 span（dial 层已经用 filter 排掉），免得 trace 量太大。
func recordFailureSpan(ctx context.Context, name string, workerID int64, err error) {
	_, span := tracer().Start(ctx, name,
		oteltrace.WithAttributes(attribute.Int64(attrWorkerID, workerID)),
	)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.End()
}
