import { Fragment, useEffect, useState } from "react";
import { ChevronRight, Cpu, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
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
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { EmptyState } from "@/components/domain/empty-state";
import { Pagination } from "@/components/domain/pagination";
import { StatusDot } from "@/components/domain/status-dot";
import { WorkerExpandPanel } from "@/components/domain/worker-expand-panel";
import {
  formatRelativeTime,
  isHeartbeatStale,
} from "@/lib/format";
import { HEARTBEAT_STALE_THRESHOLD_SEC, DEFAULT_PAGE_SIZE } from "@/lib/constants";
import { useNamespace } from "@/lib/namespace-context";
import { useWorkers } from "@/hooks/use-workers";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { cn } from "@/lib/utils";

export function WorkerTable() {
  const { namespace: selectedNamespace } = useNamespace();
  const [page, setPage] = useState(1);
  const [taskTypeDraft, setTaskTypeDraft] = useState("");
  const [appliedTaskType, setAppliedTaskType] = useState("");
  const debouncedTaskType = useDebouncedValue(taskTypeDraft.trim());
  const [expandedId, setExpandedId] = useState<number | null>(null);

  useEffect(() => {
    setAppliedTaskType(debouncedTaskType);
  }, [debouncedTaskType]);

  useEffect(() => {
    setPage(1);
    setExpandedId(null);
  }, [selectedNamespace, appliedTaskType]);

  const applySearch = () => {
    setAppliedTaskType(taskTypeDraft.trim());
    setPage(1);
  };

  const { data, isLoading, isError } = useWorkers({
    page,
    pageSize: DEFAULT_PAGE_SIZE,
    namespace: selectedNamespace ?? undefined,
    taskType: appliedTaskType || undefined,
  });

  const workers = data?.workers ?? [];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-end gap-2">
        <Input
          placeholder="任务类型"
          className="h-8 w-44"
          value={taskTypeDraft}
          onChange={(e) => setTaskTypeDraft(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && applySearch()}
        />
        <Button
          type="button"
          variant="outline"
          size="icon"
          className="h-8 w-8 shrink-0"
          aria-label="搜索工作节点"
          onClick={applySearch}
        >
          <Search className="h-4 w-4" />
        </Button>
      </div>

      {isError && (
        <p className="text-sm text-destructive">加载工作节点失败。</p>
      )}

      <Table>
        <colgroup>
          <col style={{ width: "2rem" }} />
          <col style={{ width: "2.5rem" }} />
          <col style={{ width: "38%" }} />
          <col style={{ width: "20%" }} />
          <col style={{ width: "14%" }} />
          <col style={{ width: "10%" }} />
        </colgroup>
        <TableHeader>
          <TableRow>
            <TableHead className="w-8 px-2" aria-hidden />
            <TableHead className="w-10 px-2">ID</TableHead>
            <TableHead>名称</TableHead>
            <TableHead>类型</TableHead>
            <TableHead>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="cursor-help border-b border-dotted border-muted-foreground/40">
                    心跳
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  超过 {HEARTBEAT_STALE_THRESHOLD_SEC} 秒未更新视为异常
                </TooltipContent>
              </Tooltip>
            </TableHead>
            <TableHead>成功 / 总计</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading &&
            Array.from({ length: 5 }).map((_, i) => (
              <TableRow key={i}>
                <TableCell colSpan={6}>
                  <Skeleton className="h-8 w-full" />
                </TableCell>
              </TableRow>
            ))}

          {!isLoading && workers.length === 0 && (
            <TableRow>
              <TableCell colSpan={6}>
                <EmptyState
                  icon={Cpu}
                  title="暂无工作节点"
                  description="工作节点连接队列后会自动注册。"
                />
              </TableCell>
            </TableRow>
          )}

          {workers.map((worker) => {
            const isOpen = expandedId === worker.id;
            const stale =
              worker.heartbeatTime &&
              isHeartbeatStale(worker.heartbeatTime);

            return (
              <Fragment key={worker.id}>
                <TableRow
                  className={cn(
                    "cursor-pointer",
                    isOpen && "bg-muted/40",
                  )}
                  data-state={isOpen ? "selected" : undefined}
                  aria-expanded={isOpen}
                  onClick={() =>
                    setExpandedId((prev) =>
                      prev === worker.id ? null : worker.id,
                    )
                  }
                >
                  <TableCell className="w-8 px-2">
                    <ChevronRight
                      className={cn(
                        "h-4 w-4 text-muted-foreground transition-transform duration-150",
                        isOpen && "rotate-90",
                      )}
                    />
                  </TableCell>
                  <TableCell className="px-2 text-sm tabular-nums">
                    {worker.id}
                  </TableCell>
                  <TableCell
                    className="max-w-0 truncate whitespace-nowrap"
                    title={worker.name || undefined}
                  >
                    {worker.name || "—"}
                  </TableCell>
                  <TableCell className="max-w-0 truncate text-muted-foreground">
                    {worker.taskType}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <StatusDot
                        color={
                          stale
                            ? "bg-destructive"
                            : "bg-success"
                        }
                        pulse={!!stale}
                      />
                      <span
                        className={cn(stale && "font-medium text-destructive")}
                      >
                        {formatRelativeTime(worker.heartbeatTime)}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell>
                    {worker.successDealt ?? 0} / {worker.totalDealt ?? 0}
                  </TableCell>
                </TableRow>
                {isOpen && (
                  <TableRow key={`${worker.id}-detail`}>
                    <TableCell colSpan={6} className="bg-muted/30 p-0">
                      <WorkerExpandPanel workerId={worker.id} />
                    </TableCell>
                  </TableRow>
                )}
              </Fragment>
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
    </div>
  );
}
