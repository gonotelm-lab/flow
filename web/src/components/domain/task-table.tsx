import { useEffect, useRef, useState } from "react";
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
import { copyToClipboard } from "@/lib/clipboard";
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
import { Copy, ListTodo, Search } from "lucide-react";

const STATUS_TABS = [
  { value: "all", label: "全部" },
  { value: "INITED", label: "待运行" },
  { value: "RUNNING", label: "运行中" },
  { value: "FAILED", label: "失败" },
  { value: "DONE", label: "已完成" },
  { value: "CANCELLED", label: "已取消" },
] as const;

const VALID_STATES = new Set<string>([
  "INITED",
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
  const [taskTypeDraft, setTaskTypeDraft] = useState("");
  const [taskIdDraft, setTaskIdDraft] = useState("");
  const [appliedTaskType, setAppliedTaskType] = useState("");
  const [appliedTaskId, setAppliedTaskId] = useState("");
  const debouncedTaskType = useDebouncedValue(taskTypeDraft.trim());
  const debouncedTaskId = useDebouncedValue(taskIdDraft.trim());
  const detailTaskId = searchParams.get("task");
  const [deleteTarget, setDeleteTarget] = useState<Task | null>(null);
  const skipDetailReset = useRef(true);

  useEffect(() => {
    setStatus(parseStatusParam(searchParams.get("state")));
  }, [searchParams]);

  useEffect(() => {
    setAppliedTaskType(debouncedTaskType);
    setAppliedTaskId(debouncedTaskId);
  }, [debouncedTaskType, debouncedTaskId]);

  useEffect(() => {
    setPage(1);
    if (skipDetailReset.current) {
      skipDetailReset.current = false;
      return;
    }
    if (searchParams.get("task")) {
      const next = new URLSearchParams(searchParams);
      next.delete("task");
      setSearchParams(next, { replace: true });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only reset detail when filters change
  }, [selectedNamespace, status, appliedTaskType, appliedTaskId]);

  const setDetailTaskId = (id: string | null) => {
    const next = new URLSearchParams(searchParams);
    if (id) next.set("task", id);
    else next.delete("task");
    setSearchParams(next, { replace: true });
  };

  const applySearch = () => {
    setAppliedTaskType(taskTypeDraft.trim());
    setAppliedTaskId(taskIdDraft.trim());
    setPage(1);
  };

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
    taskType: appliedTaskType || undefined,
    taskId: appliedTaskId || undefined,
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
      <div className="flex flex-wrap items-center justify-between gap-4">
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
            placeholder="任务类型"
            className="h-8 w-44"
            value={taskTypeDraft}
            onChange={(e) => setTaskTypeDraft(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && applySearch()}
          />
          <Input
            placeholder="任务 ID"
            className="h-8 w-52"
            value={taskIdDraft}
            onChange={(e) => setTaskIdDraft(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && applySearch()}
          />
          <Button
            type="button"
            variant="outline"
            size="icon"
            className="h-8 w-8 shrink-0"
            aria-label="搜索任务"
            onClick={applySearch}
          >
            <Search className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {isError && (
        <p className="text-sm text-destructive">
          加载任务失败，请检查 API 连接。
        </p>
      )}

      <Table className="table-auto">
        <colgroup>
          <col className="w-10" />
          <col style={{ minWidth: "20rem" }} />
          <col />
          <col className="w-24" />
          <col />
          <col className="w-[7.5rem]" />
        </colgroup>
        <TableHeader>
          <TableRow>
            <TableHead className="px-3" />
            <TableHead className="px-3">ID</TableHead>
            <TableHead className="px-3">类型</TableHead>
            <TableHead className="px-3">状态</TableHead>
            <TableHead className="px-3">创建时间</TableHead>
            <TableHead className="p-0 align-middle">
              <div className="flex h-11 items-center px-3 text-xs font-medium tracking-normal text-muted-foreground">
                操作
              </div>
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
                <TableCell className="px-3">
                  <StatusDot
                    color={
                      STATE_DOT_COLORS[task.state] ?? "bg-muted-foreground/50"
                    }
                    pulse={task.state === "RUNNING"}
                  />
                </TableCell>
                <TableCell className="whitespace-nowrap px-3 text-sm text-muted-foreground">
                  <div className="flex items-center gap-1.5">
                    <span>{task.id}</span>
                    <button
                      type="button"
                      className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors duration-150 hover:bg-accent hover:text-foreground"
                      aria-label="复制任务 ID"
                      onClick={(e) => {
                        e.stopPropagation();
                        copyToClipboard(task.id, "任务 ID 已复制");
                      }}
                    >
                      <Copy className="h-3 w-3" />
                    </button>
                  </div>
                </TableCell>
                <TableCell className="px-3 text-sm text-muted-foreground">
                  {task.taskType || "—"}
                </TableCell>
                <TableCell className="px-3">
                  <Badge variant={meta.variant}>{meta.label}</Badge>
                </TableCell>
                <TableCell className="whitespace-nowrap px-3 text-sm text-muted-foreground">
                  {formatTimestamp(task.createTime)}
                </TableCell>
                <TableCell className="p-0 align-middle">
                  <div
                    className="flex h-11 items-center gap-3 px-3"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <div className="flex w-8 shrink-0 items-center justify-start">
                      <button
                        type="button"
                        disabled={!canCancel}
                        className="inline-flex h-7 items-center rounded-md px-0 text-xs font-medium text-muted-foreground transition-colors duration-150 hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:opacity-40"
                        onClick={() => canCancel && handleCancel(task)}
                      >
                        取消
                      </button>
                    </div>
                    <button
                      type="button"
                      className="inline-flex h-7 items-center rounded-md px-0 text-xs font-medium text-destructive/80 transition-colors duration-150 hover:bg-destructive/8 hover:text-destructive"
                      onClick={() => setDeleteTarget(task)}
                    >
                      删除
                    </button>
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
              <span className="text-sm">{deleteTarget?.id}</span>
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
