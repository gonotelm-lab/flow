import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { toast } from "sonner";
import {
  CalendarClock,
  Check,
  Copy,
  Pause,
  Play,
  Plus,
  RotateCw,
  Search,
  Trash2,
  Zap,
} from "lucide-react";
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
import { ScheduleForm } from "@/components/domain/schedule-form";
import { StatusDot } from "@/components/domain/status-dot";
import {
  useDeleteSchedule,
  useDiscardDeadLetter,
  usePauseSchedule,
  useReplayDeadLetter,
  useResumeSchedule,
  useSchedule,
  useScheduleDeadLetters,
  useSchedules,
  useTriggerSchedule,
} from "@/hooks/use-schedules";
import { copyToClipboard } from "@/lib/clipboard";
import {
  DEAD_LETTER_INTERVALS,
  DEAD_LETTER_STATE_META,
  DEFAULT_PAGE_SIZE,
  FAST_RETRY_INTERVALS,
  FIRE_STATE_LABEL,
  SCHEDULE_STATE_META,
  SCHEDULE_TYPE_LABEL,
} from "@/lib/constants";
import { decodeProtoBytes, formatUnixMillis, truncateId } from "@/lib/format";
import { useNamespace } from "@/lib/namespace-context";
import type { Schedule } from "@/api/types";

const STATE_TABS = [
  { value: "", label: "全部" },
  { value: "SCHEDULE_STATE_ACTIVE", label: "启用中" },
  { value: "SCHEDULE_STATE_PAUSED", label: "已暂停" },
  { value: "SCHEDULE_STATE_FINISHED", label: "已完成" },
  { value: "SCHEDULE_STATE_FAILED", label: "失败" },
] as const;

const STATE_DOT: Record<string, string> = {
  SCHEDULE_STATE_ACTIVE: "bg-success",
  SCHEDULE_STATE_PAUSED: "bg-warning",
  SCHEDULE_STATE_FINISHED: "bg-muted-foreground/50",
  SCHEDULE_STATE_FAILED: "bg-destructive",
};

function triggerLabel(s: Schedule): string {
  if (s.cron) {
    return `${s.cron.expr ?? ""}${s.cron.timezone ? ` (${s.cron.timezone})` : " (UTC)"}`;
  }
  if (s.oneShot) {
    return `一次性 @ ${formatUnixMillis(s.oneShot.fireAt)}`;
  }
  return "—";
}

export function ScheduleTable() {
  const { namespace } = useNamespace();
  const [searchParams, setSearchParams] = useSearchParams();

  const state = searchParams.get("state") ?? "";
  const selectedId = searchParams.get("schedule");

  const [page, setPage] = useState(1);
  const [typeFilter, setTypeFilter] = useState("");
  const [idDraft, setIdDraft] = useState("");
  const [idFilter, setIdFilter] = useState("");
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Schedule | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Schedule | null>(null);

  const { data, isLoading } = useSchedules({
    page,
    pageSize: DEFAULT_PAGE_SIZE,
    namespace: namespace ?? undefined,
    scheduleType: typeFilter,
    state,
    id: idFilter,
  });

  const { data: detail } = useSchedule(selectedId);

  const pause = usePauseSchedule();
  const resume = useResumeSchedule();
  const trigger = useTriggerSchedule();
  const remove = useDeleteSchedule();

  const applyId = () => {
    setIdFilter(idDraft.trim());
    setPage(1);
  };

  const setTab = (value: string) => {
    const next = new URLSearchParams(searchParams);
    if (value) next.set("state", value);
    else next.delete("state");
    next.delete("schedule");
    setSearchParams(next, { replace: true });
    setPage(1);
  };

  const openDetail = (id: string) => {
    const next = new URLSearchParams(searchParams);
    next.set("schedule", id);
    setSearchParams(next, { replace: true });
  };

  const closeDetail = () => {
    const next = new URLSearchParams(searchParams);
    next.delete("schedule");
    setSearchParams(next, { replace: true });
  };

  const rows = data?.schedules ?? [];
  const totalCount = data?.page?.totalCount ?? 0;

  return (
    <div className="page-panel">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b p-3">
        <Tabs value={state} onValueChange={setTab}>
          <TabsList>
            {STATE_TABS.map((tab) => (
              <TabsTrigger key={tab.value || "all"} value={tab.value}>
                {tab.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        <div className="flex items-center gap-3">
          <select
            className="h-9 rounded-md border border-input bg-transparent px-3 text-sm"
            value={typeFilter}
            onChange={(e) => {
              setTypeFilter(e.target.value);
              setPage(1);
            }}
          >
            <option value="">全部类型</option>
            <option value="SCHEDULE_TYPE_ONE_SHOT">一次性</option>
            <option value="SCHEDULE_TYPE_CRON">重复</option>
          </select>

          <Input
            className="w-56"
            placeholder="定时任务 ID"
            value={idDraft}
            onChange={(e) => setIdDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") applyId();
            }}
          />
          <Button variant="outline" size="icon" onClick={applyId} aria-label="查询">
            <Search className="h-4 w-4" />
          </Button>

          <Button
            onClick={() => {
              setEditing(null);
              setFormOpen(true);
            }}
          >
            <Plus className="h-4 w-4" />
            新建
          </Button>
        </div>
      </div>

      <Table>
        <colgroup>
          <col style={{ width: "2.5rem" }} />
          <col style={{ minWidth: "18rem" }} />
          <col style={{ width: "7rem" }} />
          <col />
          <col style={{ minWidth: "11rem" }} />
          <col style={{ width: "7rem" }} />
          <col style={{ width: "12rem" }} />
        </colgroup>
        <TableHeader>
          <TableRow>
            <TableHead />
            <TableHead>ID</TableHead>
            <TableHead>类型</TableHead>
            <TableHead>触发规则</TableHead>
            <TableHead>下次触发</TableHead>
            <TableHead>状态</TableHead>
            <TableHead>操作</TableHead>
          </TableRow>
        </TableHeader>

        <TableBody>
          {isLoading &&
            Array.from({ length: 6 }).map((_, i) => (
              <TableRow key={i}>
                <TableCell colSpan={7} className="p-3">
                  <Skeleton className="h-6 w-full" />
                </TableCell>
              </TableRow>
            ))}

          {!isLoading && rows.length === 0 && (
            <TableRow>
              <TableCell colSpan={7} className="p-0">
                <EmptyState
                  icon={CalendarClock}
                  title="还没有定时任务"
                  description="注册一条回调，到期后由 flowserver 主动 POST 到指定地址"
                  action={{
                    label: "新建定时任务",
                    onClick: () => {
                      setEditing(null);
                      setFormOpen(true);
                    },
                  }}
                />
              </TableCell>
            </TableRow>
          )}

          {!isLoading &&
            rows.map((s) => {
              const meta = SCHEDULE_STATE_META[s.state ?? ""];
              const busy = (s.fireState ?? "FIRE_STATE_UNSPECIFIED") !== "FIRE_STATE_UNSPECIFIED";
              const canToggle =
                s.state === "SCHEDULE_STATE_ACTIVE" || s.state === "SCHEDULE_STATE_PAUSED";
              const canTrigger = canToggle && !busy;

              return (
                <TableRow key={s.id}>
                  <TableCell className="p-0">
                    <div className="flex h-11 items-center justify-center">
                      <StatusDot
                        color={STATE_DOT[s.state ?? ""] ?? "bg-muted-foreground/40"}
                        pulse={busy}
                      />
                    </div>
                  </TableCell>

                  <TableCell className="p-0">
                    <div className="flex h-11 items-center gap-2 px-3">
                      <button
                        type="button"
                        className="whitespace-nowrap font-medium hover:underline"
                        onClick={() => openDetail(s.id)}
                        title={s.id}
                      >
                        {truncateId(s.id, 12)}
                      </button>
                      <button
                        type="button"
                        className="text-muted-foreground hover:text-foreground"
                        aria-label="复制 ID"
                        onClick={() => copyToClipboard(s.id, "已复制 ID")}
                      >
                        <Copy className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </TableCell>

                  <TableCell className="p-0">
                    <div className="flex h-11 items-center px-3 text-sm text-muted-foreground">
                      {SCHEDULE_TYPE_LABEL[s.cron ? "SCHEDULE_TYPE_CRON" : "SCHEDULE_TYPE_ONE_SHOT"]}
                    </div>
                  </TableCell>

                  <TableCell className="p-0">
                    <div className="flex h-11 items-center px-3 text-sm">
                      <span className="truncate" title={triggerLabel(s)}>
                        {triggerLabel(s)}
                      </span>
                    </div>
                  </TableCell>

                  <TableCell className="p-0">
                    <div className="flex h-11 items-center px-3 text-sm tabular-nums text-muted-foreground">
                      {s.state === "SCHEDULE_STATE_PAUSED" ? "—" : formatUnixMillis(s.nextFireAt)}
                    </div>
                  </TableCell>

                  <TableCell className="p-0">
                    <div className="flex h-11 items-center gap-2 px-3">
                      <Badge variant={meta?.variant ?? "secondary"}>
                        {meta?.label ?? s.state}
                      </Badge>
                      {busy && (
                        <span className="text-xs text-muted-foreground">
                          {FIRE_STATE_LABEL[s.fireState ?? ""] ?? "投递中"}
                        </span>
                      )}
                    </div>
                  </TableCell>

                  <TableCell className="p-0">
                    <div className="flex h-11 items-center gap-3 px-3">
                      {canToggle && (
                        <Button
                          variant="ghost"
                          size="sm"
                          className="w-14"
                          onClick={() =>
                            (s.state === "SCHEDULE_STATE_ACTIVE" ? pause : resume).mutate(s.id, {
                              onSuccess: () =>
                                toast.success(
                                  s.state === "SCHEDULE_STATE_ACTIVE" ? "已暂停" : "已恢复",
                                ),
                              onError: (e) => toast.error(`操作失败：${String(e)}`),
                            })
                          }
                        >
                          {s.state === "SCHEDULE_STATE_ACTIVE" ? (
                            <>
                              <Pause className="h-3.5 w-3.5" />
                              暂停
                            </>
                          ) : (
                            <>
                              <Play className="h-3.5 w-3.5" />
                              恢复
                            </>
                          )}
                        </Button>
                      )}

                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={!canTrigger}
                        className={!canTrigger ? "opacity-40" : undefined}
                        title={
                          busy
                            ? "当前有一次投递尚未结束，需等它结束（同一 schedule 同时只允许一个 fire）"
                            : "立即投递一次，不影响 cron 节奏"
                        }
                        onClick={() =>
                          trigger.mutate(s.id, {
                            onSuccess: () => toast.success("已触发一次"),
                            onError: (e) => toast.error(`触发失败：${String(e)}`),
                          })
                        }
                      >
                        <Zap className="h-3.5 w-3.5" />
                        触发
                      </Button>

                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive"
                        onClick={() => setPendingDelete(s)}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                        删除
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              );
            })}
        </TableBody>
      </Table>

      <div className="p-3">
        <Pagination
          page={page}
          pageSize={DEFAULT_PAGE_SIZE}
          totalCount={totalCount}
          onPageChange={setPage}
        />
      </div>

      <ScheduleDetailDialog
        scheduleId={selectedId}
        schedule={detail}
        onClose={closeDetail}
        onEdit={(s) => {
          closeDetail();
          setEditing(s);
          setFormOpen(true);
        }}
      />

      <ScheduleForm
        open={formOpen}
        onOpenChange={(open) => {
          setFormOpen(open);
          if (!open) setEditing(null);
        }}
        initial={editing}
      />

      <Dialog open={!!pendingDelete} onOpenChange={(open) => !open && setPendingDelete(null)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>删除定时任务</DialogTitle>
            <DialogDescription>
              删除是硬删：任务定义、失败记录与死信会一并消失，已发出的回调无法撤回。
              服务端会把被删内容的快照写进审计日志。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPendingDelete(null)}>
              取消
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                if (!pendingDelete) return;
                remove.mutate(pendingDelete.id, {
                  onSuccess: () => {
                    toast.success("已删除");
                    setPendingDelete(null);
                  },
                  onError: (e) => toast.error(`删除失败：${String(e)}`),
                });
              }}
            >
              确认删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

// ---------------------------------------------------------------------------
// 详情
// ---------------------------------------------------------------------------

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid gap-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="break-all text-sm">{children}</span>
    </div>
  );
}

function ScheduleDetailDialog({
  scheduleId,
  schedule,
  onClose,
  onEdit,
}: {
  scheduleId: string | null;
  schedule?: Schedule;
  onClose: () => void;
  onEdit: (s: Schedule) => void;
}) {
  const replay = useReplayDeadLetter();
  const discard = useDiscardDeadLetter();

  const { data: deadLetters } = useScheduleDeadLetters(
    scheduleId ? { scheduleId, pageSize: 20 } : {},
  );

  const [copied, setCopied] = useState(false);

  useEffect(() => {
    setCopied(false);
  }, [scheduleId]);

  const rows = deadLetters?.deadLetters ?? [];

  return (
    <Dialog open={!!scheduleId} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[85vh] max-w-3xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            定时任务详情
            {schedule && (
              <Badge variant={SCHEDULE_STATE_META[schedule.state ?? ""]?.variant ?? "secondary"}>
                {SCHEDULE_STATE_META[schedule.state ?? ""]?.label ?? schedule.state}
              </Badge>
            )}
          </DialogTitle>
          <DialogDescription className="flex items-center gap-2">
            <span className="break-all font-mono text-xs">{scheduleId}</span>
            <button
              type="button"
              aria-label="复制 ID"
              className="text-muted-foreground hover:text-foreground"
              onClick={() => {
                if (!scheduleId) return;
                void copyToClipboard(scheduleId, "已复制 ID");
                setCopied(true);
              }}
            >
              {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
            </button>
          </DialogDescription>
        </DialogHeader>

        {!schedule ? (
          <Skeleton className="h-40 w-full" />
        ) : (
          <div className="grid gap-6">
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
              <Field label="命名空间">{schedule.namespace}</Field>
              <Field label="名称">{schedule.name || "—"}</Field>
              <Field label="类型">
                {SCHEDULE_TYPE_LABEL[schedule.cron ? "SCHEDULE_TYPE_CRON" : "SCHEDULE_TYPE_ONE_SHOT"]}
              </Field>
              <Field label="触发规则">{triggerLabel(schedule)}</Field>
              <Field label="下次触发">
                {schedule.state === "SCHEDULE_STATE_PAUSED"
                  ? "已暂停"
                  : formatUnixMillis(schedule.nextFireAt)}
              </Field>
              <Field label="当前 fire">
                {FIRE_STATE_LABEL[schedule.fireState ?? "FIRE_STATE_UNSPECIFIED"] ?? "—"}
              </Field>
              <Field label="回调地址">{schedule.callbackUrl}</Field>
              <Field label="Content-Type">{schedule.contentType || "—"}</Field>
              <Field label="幂等键">{schedule.idempotencyKey || "—"}</Field>
            </div>

            <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
              <Field label="连续失败">
                <span
                  className={
                    (schedule.consecutiveFailures ?? 0) > 0 ? "text-destructive" : undefined
                  }
                >
                  {schedule.consecutiveFailures ?? 0} 次
                </span>
              </Field>
              <Field label="累计失败尝试">{schedule.totalFailures ?? 0} 次</Field>
              <Field label="已跳过触发">
                <span title="因上一次投递尚未结束，或停机期间错过而丢弃的 occurrence 数">
                  {schedule.skippedCount ?? 0} 次
                </span>
              </Field>
              <Field label="上次成功">{formatUnixMillis(schedule.lastSuccessAt)}</Field>
              <Field label="创建时间">{schedule.createTime || "—"}</Field>
              <Field label="更新时间">{schedule.updateTime || "—"}</Field>
            </div>

            {schedule.lastError && (
              <div className="grid gap-1 rounded-md border border-destructive/30 bg-destructive/5 p-3">
                <span className="text-xs font-medium text-destructive">最近一次失败</span>
                <span className="break-all text-sm">{schedule.lastError}</span>
              </div>
            )}

            <div className="grid gap-1">
              <span className="text-xs text-muted-foreground">payload</span>
              <pre className="max-h-48 overflow-auto rounded-md border bg-muted/40 p-3 text-xs">
                {decodeProtoBytes(schedule.payload) || "（空）"}
              </pre>
            </div>

            <div className="grid gap-3">
              <div className="flex items-center justify-between">
                <span className="text-sm font-medium">死信</span>
                <span className="text-xs text-muted-foreground">
                  快重试 {FAST_RETRY_INTERVALS.join(" / ")} → 死信慢重试{" "}
                  {DEAD_LETTER_INTERVALS.join(" / ")}
                </span>
              </div>

              {rows.length === 0 ? (
                <p className="rounded-md border p-3 text-sm text-muted-foreground">
                  没有死信记录。
                </p>
              ) : (
                <div className="grid gap-2">
                  {rows.map((dl) => {
                    const meta = DEAD_LETTER_STATE_META[dl.state ?? ""];
                    const replayable = dl.state === "DEAD_LETTER_STATE_EXHAUSTED";
                    return (
                      <div
                        key={dl.id}
                        className="flex flex-wrap items-center justify-between gap-3 rounded-md border p-3"
                      >
                        <div className="grid gap-1">
                          <div className="flex items-center gap-2">
                            <Badge variant={meta?.variant ?? "secondary"}>
                              {meta?.label ?? dl.state}
                            </Badge>
                            <span className="text-xs text-muted-foreground">
                              计划触发 {formatUnixMillis(dl.scheduledTime)} · 已尝试{" "}
                              {dl.attemptCount ?? 0} 次
                            </span>
                          </div>
                          <span className="break-all text-xs text-muted-foreground">
                            Fire-Id {dl.fireId}
                          </span>
                          {dl.lastError && (
                            <span className="break-all text-xs text-destructive">
                              {dl.lastError}
                            </span>
                          )}
                        </div>

                        <div className="flex items-center gap-2">
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={!replayable}
                            className={!replayable ? "opacity-40" : undefined}
                            title="重投会沿用原 Fire-Id，接收方据此判断是补投而非新事件"
                            onClick={() =>
                              replay.mutate(dl.id, {
                                onSuccess: () => toast.success("已发起重投"),
                                onError: (e) => toast.error(`重投失败：${String(e)}`),
                              })
                            }
                          >
                            <RotateCw className="h-3.5 w-3.5" />
                            重投
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={!replayable}
                            className={!replayable ? "opacity-40" : undefined}
                            onClick={() =>
                              discard.mutate(dl.id, {
                                onSuccess: () => toast.success("已放弃"),
                                onError: (e) => toast.error(`放弃失败：${String(e)}`),
                              })
                            }
                          >
                            放弃
                          </Button>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            关闭
          </Button>
          {schedule && <Button onClick={() => onEdit(schedule)}>编辑</Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
