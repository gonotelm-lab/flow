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

// ---------------------------------------------------------------------------
// 定时回调（Schedule）
//
// 注意枚举值带枚举名前缀：proto3 的枚举值在包级可见，不加前缀会和同包的
// TaskState（已有 FAILED 等值）重名。JSON 里出现的就是这些带前缀的名字。
// ---------------------------------------------------------------------------

export const SCHEDULE_TYPES = [
  "SCHEDULE_TYPE_UNSPECIFIED",
  "SCHEDULE_TYPE_ONE_SHOT",
  "SCHEDULE_TYPE_CRON",
] as const;

export const SCHEDULE_TYPE_LABEL: Record<string, string> = {
  SCHEDULE_TYPE_UNSPECIFIED: "未指定",
  SCHEDULE_TYPE_ONE_SHOT: "一次性",
  SCHEDULE_TYPE_CRON: "重复",
};

export const SCHEDULE_STATES = [
  "SCHEDULE_STATE_UNSPECIFIED",
  "SCHEDULE_STATE_ACTIVE",
  "SCHEDULE_STATE_PAUSED",
  "SCHEDULE_STATE_FINISHED",
  "SCHEDULE_STATE_FAILED",
] as const;

export const SCHEDULE_STATE_META: Record<
  string,
  {
    label: string;
    variant: "secondary" | "warning" | "success" | "destructive";
  }
> = {
  SCHEDULE_STATE_ACTIVE: { label: "启用中", variant: "success" },
  SCHEDULE_STATE_PAUSED: { label: "已暂停", variant: "warning" },
  SCHEDULE_STATE_FINISHED: { label: "已完成", variant: "secondary" },
  SCHEDULE_STATE_FAILED: { label: "失败", variant: "destructive" },
};

/** fire 槽位状态。UNSPECIFIED 表示当前没有未完成的 fire。 */
export const FIRE_STATE_LABEL: Record<string, string> = {
  FIRE_STATE_UNSPECIFIED: "空闲",
  FIRE_STATE_PENDING: "待投递",
  FIRE_STATE_DELIVERING: "投递中",
  FIRE_STATE_DEAD_LETTER: "死信重试中",
};

export const DEAD_LETTER_STATES = [
  "DEAD_LETTER_STATE_UNSPECIFIED",
  "DEAD_LETTER_STATE_RETRYING",
  "DEAD_LETTER_STATE_EXHAUSTED",
  "DEAD_LETTER_STATE_REPLAYED",
  "DEAD_LETTER_STATE_DISCARDED",
] as const;

export const DEAD_LETTER_STATE_META: Record<
  string,
  {
    label: string;
    variant: "secondary" | "warning" | "success" | "destructive";
  }
> = {
  DEAD_LETTER_STATE_RETRYING: { label: "重试中", variant: "warning" },
  DEAD_LETTER_STATE_EXHAUSTED: { label: "待处理", variant: "destructive" },
  DEAD_LETTER_STATE_REPLAYED: { label: "已重投", variant: "success" },
  DEAD_LETTER_STATE_DISCARDED: { label: "已放弃", variant: "secondary" },
};

/**
 * 调度器的重试节奏，用于在界面上解释「为什么它还在重试」。
 * 数值与 server/etc/conf.toml.tpl 的默认值一致。
 */
export const FAST_RETRY_INTERVALS = ["1s", "2s", "4s", "8s", "16s"];
export const DEAD_LETTER_INTERVALS = ["1m", "5m", "30m"];
