package runtime

// worker 运行时用到的 tracer 和 span attribute 常量。
const (
	tracerName = "flow.worker"

	attrTaskID        = "flow.task.id"
	attrTaskNamespace = "flow.task.namespace"
	attrTaskType      = "flow.task.type"
	attrWorkerID      = "flow.worker.id"
)
