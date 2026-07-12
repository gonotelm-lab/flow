import { Skeleton } from "@/components/ui/skeleton";
import { formatRelativeTime } from "@/lib/format";
import { useWorker } from "@/hooks/use-workers";

type WorkerExpandPanelProps = {
  workerId: number;
};

export function WorkerExpandPanel({ workerId }: WorkerExpandPanelProps) {
  const { data: worker, isLoading } = useWorker(workerId);

  if (isLoading || !worker) {
    return (
      <div className="space-y-2 p-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-12 w-full" />
      </div>
    );
  }

  return (
    <div className="grid grid-cols-2 gap-4 p-4 text-sm sm:grid-cols-4">
      <Field label="ID" value={String(worker.id)} />
      <Field label="名称" value={worker.name || "—"} />
      <Field label="命名空间" value={worker.namespace} />
      <Field label="任务类型" value={worker.taskType} />
      <Field label="创建时间" value={formatRelativeTime(worker.createTime)} />
      <Field label="最近工作" value={formatRelativeTime(worker.lastWorkTime)} />
      <Field label="心跳" value={formatRelativeTime(worker.heartbeatTime)} />
      <Field
        label="处理量"
        value={`${worker.successDealt ?? 0} / ${worker.totalDealt ?? 0}`}
      />
    </div>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="font-medium">{value}</dd>
    </div>
  );
}
