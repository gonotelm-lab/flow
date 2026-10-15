import { useEffect, useState } from "react";
import { toast } from "sonner";
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
import { useCreateSchedule, useUpdateSchedule } from "@/hooks/use-schedules";
import { useNamespace } from "@/lib/namespace-context";
import type { Schedule } from "@/api/types";

type ScheduleFormProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** 传入表示编辑，null / undefined 表示新建 */
  initial?: Schedule | null;
};

type TriggerKind = "one_shot" | "cron";

/**
 * proto 的 bytes 字段在 protojson 里是 base64，所以用户输入的文本要按 UTF-8 编码后再 base64。
 */
function encodeUtf8Base64(text: string): string {
  const bytes = new TextEncoder().encode(text);
  let binary = "";
  bytes.forEach((b) => {
    binary += String.fromCharCode(b);
  });
  return btoa(binary);
}

/** datetime-local 的 "YYYY-MM-DDTHH:mm" 按本地时区解析成 unix 毫秒。 */
function localDateTimeToMillis(value: string): number | null {
  if (!value) return null;
  const ms = new Date(value).getTime();
  return Number.isNaN(ms) ? null : ms;
}

function millisToLocalDateTime(ms?: string | number): string {
  if (ms === undefined || ms === null || ms === "") return "";
  const n = typeof ms === "string" ? Number(ms) : ms;
  if (!Number.isFinite(n) || n <= 0) return "";
  const d = new Date(n);
  const pad = (v: number) => String(v).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function ScheduleForm({ open, onOpenChange, initial }: ScheduleFormProps) {
  const { namespace } = useNamespace();
  const create = useCreateSchedule();
  const update = useUpdateSchedule();

  const editing = !!initial;

  const [kind, setKind] = useState<TriggerKind>("cron");
  const [fireAt, setFireAt] = useState("");
  const [cronExpr, setCronExpr] = useState("*/5 * * * *");
  const [timezone, setTimezone] = useState("");
  const [callbackUrl, setCallbackUrl] = useState("");
  const [contentType, setContentType] = useState("application/json");
  const [payload, setPayload] = useState("");
  const [name, setName] = useState("");
  const [idempotencyKey, setIdempotencyKey] = useState("");

  // 打开时把 initial 灌进表单
  useEffect(() => {
    if (!open) return;

    if (initial) {
      setName(initial.name ?? "");
      setCallbackUrl(initial.callbackUrl ?? "");
      setContentType(initial.contentType ?? "application/json");
      setIdempotencyKey(initial.idempotencyKey ?? "");
      // 编辑时不回填 payload 会让「保存」把原有 payload 清空，因此这里展示占位提示
      setPayload("");
      if (initial.cron) {
        setKind("cron");
        setCronExpr(initial.cron.expr ?? "*/5 * * * *");
        setTimezone(initial.cron.timezone ?? "");
        setFireAt("");
      } else if (initial.oneShot) {
        setKind("one_shot");
        setFireAt(millisToLocalDateTime(initial.oneShot.fireAt));
        setCronExpr("*/5 * * * *");
        setTimezone("");
      }
    } else {
      setKind("cron");
      setCronExpr("*/5 * * * *");
      setTimezone("");
      setFireAt("");
      setCallbackUrl("");
      setContentType("application/json");
      setPayload("");
      setName("");
      setIdempotencyKey("");
    }
  }, [open, initial]);

  const submitting = create.isPending || update.isPending;

  const submit = () => {
    const trimmedUrl = callbackUrl.trim();
    if (!trimmedUrl) {
      toast.error("请填写回调地址");
      return;
    }

    const schedule: Schedule = {
      id: initial?.id ?? "",
      namespace: initial?.namespace ?? namespace ?? "",
      name: name.trim(),
      callbackUrl: trimmedUrl,
      contentType: contentType.trim() || "application/octet-stream",
      payload: encodeUtf8Base64(payload),
    };

    if (kind === "one_shot") {
      const ms = localDateTimeToMillis(fireAt);
      if (ms === null) {
        toast.error("请选择触发时间");
        return;
      }
      if (ms <= Date.now()) {
        toast.error("触发时间必须晚于当前时间");
        return;
      }
      schedule.oneShot = { fireAt: ms };
    } else {
      const expr = cronExpr.trim();
      if (expr.split(/\s+/).length !== 5) {
        toast.error("cron 需要标准 5 字段：分 时 日 月 周");
        return;
      }
      schedule.cron = { expr, timezone: timezone.trim() };
    }

    if (!editing && idempotencyKey.trim()) {
      schedule.idempotencyKey = idempotencyKey.trim();
    }

    const onError = (err: unknown) => {
      const message = err instanceof Error ? err.message : String(err);
      toast.error(`保存失败：${message}`);
    };

    if (editing) {
      update.mutate(
        { id: initial!.id, schedule },
        {
          onSuccess: () => {
            toast.success("已保存");
            onOpenChange(false);
          },
          onError,
        },
      );
    } else {
      create.mutate(schedule, {
        onSuccess: () => {
          toast.success("已创建");
          onOpenChange(false);
        },
        onError,
      });
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{editing ? "编辑定时任务" : "新建定时任务"}</DialogTitle>
          <DialogDescription>
            到期后 flowserver 会向回调地址发一次 POST，body 就是下面的 payload。
            投递是至少一次，接收方请用 X-Flow-Fire-Id 做幂等去重。
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4">
          <div className="grid gap-2">
            <label className="text-sm font-medium">触发方式</label>
            <div className="flex gap-2">
              <Button
                type="button"
                variant={kind === "one_shot" ? "default" : "outline"}
                size="sm"
                onClick={() => setKind("one_shot")}
              >
                一次性
              </Button>
              <Button
                type="button"
                variant={kind === "cron" ? "default" : "outline"}
                size="sm"
                onClick={() => setKind("cron")}
              >
                重复（cron）
              </Button>
            </div>
          </div>

          {kind === "one_shot" ? (
            <div className="grid gap-2">
              <label className="text-sm font-medium">触发时间</label>
              <Input
                type="datetime-local"
                value={fireAt}
                onChange={(e) => setFireAt(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                必须晚于当前时间，服务端会用数据库时间校验并拒绝过期注册。
              </p>
            </div>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <label className="text-sm font-medium">cron 表达式</label>
                <Input
                  value={cronExpr}
                  onChange={(e) => setCronExpr(e.target.value)}
                  placeholder="*/5 * * * *"
                />
              </div>
              <div className="grid gap-2">
                <label className="text-sm font-medium">时区（IANA）</label>
                <Input
                  value={timezone}
                  onChange={(e) => setTimezone(e.target.value)}
                  placeholder="UTC"
                />
              </div>
              <p className="text-xs text-muted-foreground sm:col-span-2">
                标准 5 字段「分 时 日 月 周」，最小粒度 1 分钟。星期 0-6 且
                <span className="font-medium"> 0 表示周日</span>，月份 1-12；
                不支持 L / W / #。这与 Quartz 不同，请勿按 Quartz 语义书写。
                留空时区表示 UTC。
              </p>
            </div>
          )}

          <div className="grid gap-2">
            <label className="text-sm font-medium">回调地址</label>
            <Input
              value={callbackUrl}
              onChange={(e) => setCallbackUrl(e.target.value)}
              placeholder="https://example.com/hook"
            />
            <p className="text-xs text-muted-foreground">
              仅支持 http / https；不探测可达性，也不会跟随重定向。
            </p>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <label className="text-sm font-medium">名称（可选）</label>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="便于运维识别"
              />
            </div>
            <div className="grid gap-2">
              <label className="text-sm font-medium">Content-Type</label>
              <Input
                value={contentType}
                onChange={(e) => setContentType(e.target.value)}
                placeholder="application/json"
              />
            </div>
          </div>

          {!editing && (
            <div className="grid gap-2">
              <label className="text-sm font-medium">幂等键（可选）</label>
              <Input
                value={idempotencyKey}
                onChange={(e) => setIdempotencyKey(e.target.value)}
                placeholder="填了之后重复提交只会创建一条"
              />
            </div>
          )}

          <div className="grid gap-2">
            <label className="text-sm font-medium">
              payload{editing && <span className="text-muted-foreground">（留空则保持原值）</span>}
            </label>
            <textarea
              className="min-h-24 w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
              value={payload}
              onChange={(e) => setPayload(e.target.value)}
              placeholder='{"hello":"world"}'
            />
            <p className="text-xs text-muted-foreground">
              这段文本会原样作为回调 body 发送，不做任何包装。
            </p>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={submit} disabled={submitting}>
            {submitting ? "保存中…" : "保存"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
