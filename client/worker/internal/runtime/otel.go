// flow/client/worker/internal/runtime/otel.go
package runtime

// worker 运行时使用的 tracer 与 span attribute 常量。
const (
	tracerName = "flow.worker"

	attrTaskID        = "flow.task.id"
	attrTaskNamespace = "flow.task.namespace"
	attrTaskType      = "flow.task.type"
	attrWorkerID      = "flow.worker.id"
)
