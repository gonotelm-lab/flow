import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Info } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useCreateSchedule } from "@/hooks/use-schedules";
import { useNamespace } from "@/lib/namespace-context";
import type { Schedule } from "@/api/types";

type ScheduleFormProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
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

/** 打开对话框时给一个随机幂等键：用户不改就用它，改了就用用户填的。 */
function randomIdempotencyKey(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

/** 字段标签 + 悬停说明。说明放 tooltip，不占表单的纵向空间。 */
function FieldLabel({ text, hint }: { text: string; hint: string }) {
  return (
    <div className="flex items-center gap-1.5">
      <label className="text-sm font-medium">{text}</label>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label={`${text}说明`}
            className="text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <Info className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-xs leading-relaxed">
          {hint}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}

export function ScheduleForm({ open, onOpenChange }: ScheduleFormProps) {
  const { namespace } = useNamespace();
  const create = useCreateSchedule();

  const [kind, setKind] = useState<TriggerKind>("cron");
  const [fireAt, setFireAt] = useState("");
  const [cronExpr, setCronExpr] = useState("*/5 * * * *");
  const [timezone, setTimezone] = useState("");
  const [callbackUrl, setCallbackUrl] = useState("");
  const [contentType, setContentType] = useState("application/json");
  const [payload, setPayload] = useState("");
  const [name, setName] = useState("");
  const [idempotencyKey, setIdempotencyKey] = useState("");

  // 每次打开都从空白开始：定义注册之后改不了，只能新建一条
  useEffect(() => {
    if (!open) return;

    setKind("cron");
    setCronExpr("*/5 * * * *");
    setTimezone("");
    setFireAt("");
    setCallbackUrl("");
    setContentType("application/json");
    setPayload("");
    setName("");
    setIdempotencyKey(randomIdempotencyKey());
  }, [open]);

  const submit = () => {
    const trimmedUrl = callbackUrl.trim();
    if (!trimmedUrl) {
      toast.error("请填写回调地址");
      return;
    }

    const schedule: Schedule = {
      // 定义里没有 id：客户端留空，由服务端生成
      id: "",
      namespace: namespace ?? "",
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

    if (idempotencyKey.trim()) {
      schedule.idempotencyKey = idempotencyKey.trim();
    }

    create.mutate(schedule, {
      onSuccess: () => {
        toast.success("已创建");
        onOpenChange(false);
      },
      onError: (err: unknown) => {
        const message = err instanceof Error ? err.message : String(err);
        toast.error(`创建失败：${message}`);
      },
    });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        aria-describedby={undefined}
        className="max-h-[85vh] max-w-2xl overflow-y-auto"
      >
        <DialogHeader>
          <DialogTitle>新建定时任务</DialogTitle>
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
              <FieldLabel
                text="触发时间"
                hint="必须晚于当前时间。"
              />
              <Input
                type="datetime-local"
                value={fireAt}
                onChange={(e) => setFireAt(e.target.value)}
              />
            </div>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <FieldLabel
                  text="cron 表达式"
                  hint="标准 5 字段（分 时 日 月 周），最小粒度 1 分钟。星期 0-6，0 为周日；月份 1-12；不支持 L / W / #。语义与 Quartz 不同。"
                />
                <Input
                  value={cronExpr}
                  onChange={(e) => setCronExpr(e.target.value)}
                  placeholder="*/5 * * * *"
                />
              </div>
              <div className="grid gap-2">
                <FieldLabel text="时区（IANA）" hint="留空表示 UTC。" />
                <Input
                  value={timezone}
                  onChange={(e) => setTimezone(e.target.value)}
                  placeholder="UTC"
                />
              </div>
            </div>
          )}

          <div className="grid gap-2">
            <FieldLabel
              text="回调地址"
              hint="仅支持 http / https。"
            />
            <Input
              value={callbackUrl}
              onChange={(e) => setCallbackUrl(e.target.value)}
              placeholder="https://example.com/hook"
            />
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <FieldLabel text="名称（可选）" hint="便于运维识别。" />
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="便于运维识别"
              />
            </div>
            <div className="grid gap-2">
              <FieldLabel
                text="Content-Type"
                hint="留空表示 application/octet-stream。"
              />
              <Input
                value={contentType}
                onChange={(e) => setContentType(e.target.value)}
                placeholder="application/json"
              />
            </div>
          </div>

          <div className="grid gap-2">
            <FieldLabel
              text="幂等键（可选）"
              hint="已自动生成随机键；同一个键重复提交只保留一条。"
            />
            <Input
              value={idempotencyKey}
              onChange={(e) => setIdempotencyKey(e.target.value)}
              placeholder="自动生成"
            />
          </div>

          <div className="grid gap-2">
            <FieldLabel
              text="payload"
              hint="作为回调请求体原样发送。"
            />
            <textarea
              className="min-h-24 w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
              value={payload}
              onChange={(e) => setPayload(e.target.value)}
              placeholder='{"hello":"world"}'
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={submit} disabled={create.isPending}>
            {create.isPending ? "创建中…" : "创建"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
