export const TASK_STATES = [
  "TASK_STATE_UNSPECIFIED",
  "INITED",
  "RUNNING",
  "DONE",
  "FAILED",
  "CANCELLED",
] as const;

export type TaskState = (typeof TASK_STATES)[number];

export const TASK_STATE_META: Record<
  string,
  { label: string; variant: "secondary" | "warning" | "success" | "destructive" }
> = {
  INITED: { label: "已初始化", variant: "secondary" },
  RUNNING: { label: "运行中", variant: "warning" },
  DONE: { label: "已完成", variant: "success" },
  FAILED: { label: "失败", variant: "destructive" },
  CANCELLED: { label: "已取消", variant: "secondary" },
};

export const DEFAULT_PAGE_SIZE = 20;

export const HEARTBEAT_STALE_THRESHOLD_SEC = 30;
