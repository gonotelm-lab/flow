import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { EmptyState } from "@/components/domain/empty-state";
import { Pagination } from "@/components/domain/pagination";
import { STATE_DOT_COLORS, StatusDot } from "@/components/domain/status-dot";
import { TaskDetailPanel } from "@/components/domain/task-detail-panel";
import { DEFAULT_PAGE_SIZE } from "@/lib/constants";
import { useNamespace } from "@/lib/namespace-context";
import {
  formatTimestamp,
  formatTaskState,
} from "@/lib/format";
import {
  useCancelTask,
  useDeleteTask,
  useTasks,
} from "@/hooks/use-tasks";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import type { Task } from "@/api/types";
import { ListTodo } from "lucide-react";

const STATUS_TABS = [
  { value: "all", label: "全部" },
  { value: "RUNNING", label: "运行中" },
  { value: "FAILED", label: "失败" },
  { value: "DONE", label: "已完成" },
  { value: "CANCELLED", label: "已取消" },
] as const;

const VALID_STATES = new Set<string>([
  "RUNNING",
  "FAILED",
  "DONE",
  "CANCELLED",
]);

function parseStatusParam(param: string | null): string {
  return param && VALID_STATES.has(param) ? param : "all";
}

export function TaskTable() {
  const { namespace: selectedNamespace } = useNamespace();
  const [searchParams, setSearchParams] = useSearchParams();
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState(() =>
    parseStatusParam(searchParams.get("state")),
  );
  const [taskType, setTaskType] = useState("");
  const [taskId, setTaskId] = useState("");
  const debouncedTaskType = useDebouncedValue(taskType.trim());
  const debouncedTaskId = useDebouncedValue(taskId.trim());
  const [detailTaskId, setDetailTaskId] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Task | null>(null);

  useEffect(() => {
    setStatus(parseStatusParam(searchParams.get("state")));
  }, [searchParams]);

  useEffect(() => {
    setPage(1);
    setDetailTaskId(null);
  }, [selectedNamespace, status, debouncedTaskType, debouncedTaskId]);

  const handleStatusChange = (v: string) => {
    setStatus(v);
    const next = new URLSearchParams(searchParams);
    if (v === "all") next.delete("state");
    else next.set("state", v);
    setSearchParams(next, { replace: true });
  };

  const filters = {
    page,
    pageSize: DEFAULT_PAGE_SIZE,
    namespace: selectedNamespace ?? undefined,
    taskType: debouncedTaskType || undefined,
    taskId: debouncedTaskId || undefined,
    state: status === "all" ? undefined : status,
  };

  const { data, isLoading, isError } = useTasks(filters);
  const cancelMutation = useCancelTask();
  const deleteMutation = useDeleteTask();

  const handleCancel = async (task: Task) => {
    try {
      await cancelMutation.mutateAsync(task.id);
      toast.success("任务已取消");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "取消失败");
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      await deleteMutation.mutateAsync(deleteTarget.id);
      toast.success("任务已删除");
      setDeleteTarget(null);
      if (detailTaskId === deleteTarget.id) setDetailTaskId(null);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "删除失败");
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Tabs value={status} onValueChange={handleStatusChange}>
          <TabsList>
            {STATUS_TABS.map((t) => (
              <TabsTrigger key={t.value} value={t.value}>
                {t.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <div className="flex flex-wrap items-center gap-2">
          <Input
            placeholder="任务类型（支持模糊匹配）"
            className="h-8 w-44"
            value={taskType}
            onChange={(e) => setTaskType(e.target.value)}
          />
          <Input
            placeholder="任务 ID（支持部分匹配）"
            className="h-8 w-52 font-mono text-xs"
            value={taskId}
            onChange={(e) => setTaskId(e.target.value)}
          />
        </div>
      </div>

      {isError && (
        <p className="text-sm text-destructive">
          加载任务失败，请在设置中检查 API 连接。
        </p>
      )}

      <Table>
        <colgroup>
          <col style={{ width: "2.5rem" }} />
          <col />
          <col style={{ width: "14%" }} />
          <col style={{ width: "10%" }} />
          <col style={{ width: "18%" }} />
          <col style={{ width: "8.5rem" }} />
        </colgroup>
        <TableHeader>
          <TableRow>
            <TableHead className="w-10 px-3" />
            <TableHead className="px-3">ID</TableHead>
            <TableHead className="px-3">类型</TableHead>
            <TableHead className="px-3">状态</TableHead>
            <TableHead className="px-3">创建时间</TableHead>
            <TableHead className="px-3">
              <div className="flex h-7 items-center">操作</div>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading &&
            Array.from({ length: 6 }).map((_, i) => (
              <TableRow key={i}>
                <TableCell colSpan={6}>
                  <Skeleton className="h-8 w-full" />
                </TableCell>
              </TableRow>
            ))}

          {!isLoading && data?.tasks.length === 0 && (
            <TableRow>
              <TableCell colSpan={6}>
                <EmptyState
                  icon={ListTodo}
                  title="暂无任务"
                  description="提交到队列后，任务将显示在这里。"
                />
              </TableCell>
            </TableRow>
          )}

          {data?.tasks.map((task) => {
            const meta = formatTaskState(task.state);
            const canCancel =
              task.state === "INITED" || task.state === "RUNNING";

            return (
              <TableRow
                key={task.id}
                className="cursor-pointer"
                onClick={() => setDetailTaskId(task.id)}
              >
                <TableCell className="w-10 px-3">
                  <StatusDot
                    color={STATE_DOT_COLORS[task.state] ?? "bg-zinc-500"}
                    pulse={task.state === "RUNNING"}
                  />
                </TableCell>
                <TableCell className="truncate px-3 font-mono text-xs">
                  {task.id}
                </TableCell>
                <TableCell className="truncate px-3 text-muted-foreground">
                  {task.taskType || "—"}
                </TableCell>
                <TableCell className="px-3">
                  <Badge variant={meta.variant}>{meta.label}</Badge>
                </TableCell>
                <TableCell className="whitespace-nowrap px-3 text-muted-foreground">
                  {formatTimestamp(task.createTime)}
                </TableCell>
                <TableCell className="px-3">
                  <div
                    className="flex h-7 items-center gap-2"
                    onClick={(e) => e.stopPropagation()}
                  >
                    {canCancel && (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 !px-0 text-xs hover:bg-transparent"
                        onClick={() => handleCancel(task)}
                      >
                        取消
                      </Button>
                    )}
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-7 !px-0 text-xs text-destructive hover:bg-transparent"
                      onClick={() => setDeleteTarget(task)}
                    >
                      删除
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>

      {data?.page && (
        <Pagination
          page={data.page.page}
          pageSize={data.page.pageSize}
          totalCount={Number(data.page.totalCount)}
          onPageChange={setPage}
        />
      )}

      <Dialog
        open={!!detailTaskId}
        onOpenChange={(open) => !open && setDetailTaskId(null)}
      >
        <DialogContent className="flex max-h-[85vh] max-w-4xl flex-col gap-0 overflow-hidden p-0 sm:max-w-4xl">
          <DialogHeader className="shrink-0 border-b border-border px-6 py-4 text-left">
            <DialogTitle>任务详情</DialogTitle>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-y-auto px-6 py-4">
            {detailTaskId && <TaskDetailPanel taskId={detailTaskId} />}
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={!!deleteTarget} onOpenChange={() => setDeleteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除任务？</DialogTitle>
            <DialogDescription>
              将永久删除任务{" "}
              <code className="font-mono text-xs">{deleteTarget?.id}</code>
              ，此操作不可撤销。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>
              取消
            </Button>
            <Button variant="destructive" onClick={handleDelete}>
              删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
