import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import {
  formatTaskState,
  decodeProtoBytes,
  formatTimestamp,
  formatUnixMillis,
} from "@/lib/format";
import { useTask, useTaskEvents } from "@/hooks/use-tasks";
import type { TaskEvent } from "@/api/types";

type TaskDetailPanelProps = {
  taskId: string;
};

export function TaskDetailPanel({ taskId }: TaskDetailPanelProps) {
  const { data: task, isLoading } = useTask(taskId);
  const { data: events } = useTaskEvents(taskId);

  if (isLoading || !task) {
    return (
      <div className="space-y-3 py-2">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-20 w-full" />
        <Skeleton className="h-32 w-full" />
      </div>
    );
  }

  const meta = formatTaskState(task.state);

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center gap-3">
        <Badge variant={meta.variant}>{meta.label}</Badge>
        <code className="break-all font-mono text-xs text-muted-foreground">
          {task.id}
        </code>
      </div>

      <div className="grid grid-cols-2 gap-x-6 gap-y-3 text-sm sm:grid-cols-3 lg:grid-cols-4">
        <Meta label="命名空间" value={task.namespace} />
        <Meta label="任务类型" value={task.taskType || "—"} />
        <Meta label="重试次数" value={String(task.attemptNo ?? 0)} />
        <Meta label="工作节点 ID" value={String(task.workerId ?? "—")} />
        <Meta label="最大重试" value={String(task.maxRetry ?? 0)} />
        <Meta label="创建时间" value={formatTimestamp(task.createTime)} />
        <Meta label="更新时间" value={formatTimestamp(task.updateTime)} />
        <Meta label="心跳" value={formatTimestamp(task.lastHeartbeatTime)} />
      </div>

      <BytesBlock label="载荷" value={task.payload} defaultOpen={false} />
      <BytesBlock label="结果" value={task.result} defaultOpen={false} />
      <BytesBlock label="错误" value={task.error} defaultOpen={false} />

      <div>
        <h4 className="mb-2 text-xs font-medium text-muted-foreground">
          事件
        </h4>
        <TaskEventsTimeline events={events?.events ?? []} />
      </div>
    </div>
  );
}

function TaskEventsTimeline({ events }: { events: TaskEvent[] }) {
  if (events.length === 0) {
    return <p className="text-sm text-muted-foreground">暂无事件</p>;
  }

  return (
    <TooltipProvider delayDuration={200}>
      <div className="overflow-x-auto rounded-md border border-border bg-muted/20 px-3 py-3">
        <div className="flex min-w-min items-center gap-1.5">
          {events.map((ev, index) => (
            <div key={ev.id} className="flex items-center gap-1.5">
              {index > 0 && (
                <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60" />
              )}
              <EventChip event={ev} />
            </div>
          ))}
        </div>
      </div>
    </TooltipProvider>
  );
}

const EVENT_VARIANT: Record<
  string,
  "secondary" | "warning" | "success" | "destructive"
> = {
  INITED: "secondary",
  RUNNING: "warning",
  DONE: "success",
  FAILED: "destructive",
  RETRIED: "secondary",
  STALE_DETECTED: "destructive",
  CANCELLED: "secondary",
};

function EventChip({ event }: { event: TaskEvent }) {
  const description = event.payload
    ? decodeProtoBytes(event.payload)
    : "无描述";

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          className="rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Badge
            variant={EVENT_VARIANT[event.eventType] ?? "secondary"}
            className="cursor-default font-mono text-[11px] hover:opacity-90"
          >
            {event.eventType}
          </Badge>
        </button>
      </TooltipTrigger>
      <TooltipContent side="top" className="max-w-xs space-y-1.5">
        <p className="font-mono text-[11px] text-muted-foreground">
          {formatUnixMillis(event.createTime)}
        </p>
        <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-all font-mono text-[11px] leading-relaxed">
          {description}
        </pre>
      </TooltipContent>
    </Tooltip>
  );
}

function Meta({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="font-medium">{value}</dd>
    </div>
  );
}

function BytesBlock({
  label,
  value,
  defaultOpen = true,
}: {
  label: string;
  value?: string;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  if (!value) return null;

  return (
    <div className="overflow-hidden rounded-md border border-border">
      <button
        type="button"
        className="flex w-full items-center justify-between px-3 py-2 text-xs font-medium text-muted-foreground hover:bg-muted/30"
        onClick={() => setOpen((v) => !v)}
      >
        <span>{label}</span>
        <ChevronDown
          className={cn(
            "h-3.5 w-3.5 shrink-0 transition-transform",
            open && "rotate-180",
          )}
        />
      </button>
      {open && (
        <pre className="max-h-56 overflow-auto border-t border-border bg-muted/30 p-3 font-mono text-xs">
          {decodeProtoBytes(value)}
        </pre>
      )}
    </div>
  );
}
