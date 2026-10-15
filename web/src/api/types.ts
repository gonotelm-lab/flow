export interface PageRequest {
  page?: number;
  page_size?: number;
}

/** protojson JSON field names (camelCase) */
export interface PageResponse {
  page: number;
  pageSize: number;
  totalCount: number;
}

export interface Namespace {
  name: string;
  description?: string;
  creator?: string;
  createTime?: string;
  updateTime?: string;
  apiKey?: string;
  apiKeyPreview?: string;
}

export interface Task {
  id: string;
  namespace: string;
  taskType: string;
  /** proto bytes → JSON base64 string */
  payload?: string;
  /** proto bytes → JSON base64 string */
  result?: string;
  /** proto bytes → JSON base64 string */
  error?: string;
  state: string;
  createTime?: string;
  updateTime?: string;
  nextRunTime?: number;
  maxRetry?: number;
  attemptNo?: number;
  workerId?: number;
  lastHeartbeatTime?: string;
}

export interface TaskEvent {
  id: number;
  taskId: string;
  eventType: string;
  createTime?: string | number;
  /** proto bytes → JSON base64 string */
  payload?: string;
}

export interface Worker {
  id: number;
  name?: string;
  namespace: string;
  taskType: string;
  createTime?: string;
  heartbeatTime?: string;
  lastWorkTime?: string;
  totalDealt?: number;
  successDealt?: number;
}

export interface ListNamespacesResponse {
  page: PageResponse;
  namespaces: Namespace[];
}

export interface ListTasksResponse {
  page: PageResponse;
  tasks: Task[];
}

export interface ListWorkersResponse {
  page: PageResponse;
  workers: Worker[];
}

export interface ListTaskEventsResponse {
  page: PageResponse;
  events: TaskEvent[];
}

// ---------------------------------------------------------------------------
// 定时回调（Schedule）
//
// 触发类型不在 Schedule 上单独占字段，而是由 oneof 分支表达：服务端只会在
// oneShot / cron 里给一个。int64 字段 protojson 会序列化成字符串，读的时候两种都要认。
// ---------------------------------------------------------------------------

export interface OneShotTrigger {
  /** unix 毫秒 */
  fireAt?: string | number;
}

export interface CronTrigger {
  /** 标准 5 字段（分 时 日 月 周） */
  expr?: string;
  /** IANA 时区名，缺省 UTC */
  timezone?: string;
}

export interface Schedule {
  id: string;
  namespace: string;
  name?: string;
  oneShot?: OneShotTrigger;
  cron?: CronTrigger;
  callbackUrl: string;
  contentType?: string;
  /** proto bytes → JSON base64 string */
  payload?: string;
  idempotencyKey?: string;
  /** SCHEDULE_STATE_* */
  state?: string;
  /** unix 毫秒 */
  nextFireAt?: string | number;
  /** FIRE_STATE_*；UNSPECIFIED 表示当前没有未完成的 fire */
  fireState?: string;
  attemptNo?: number;
  /** unix 毫秒 */
  lastSuccessAt?: string | number;
  consecutiveFailures?: number;
  totalFailures?: string | number;
  /** 因上一次 fire 未结束或迟到而跳过的 occurrence 数 */
  skippedCount?: string | number;
  lastError?: string;
  createTime?: string;
  updateTime?: string;
}

export interface ScheduleDeadLetter {
  id: string;
  scheduleId: string;
  namespace: string;
  /** 同一次 fire 的所有重试与重投都用这个 id，接收方据此去重 */
  fireId: string;
  scheduledTime?: string | number;
  attemptCount?: number;
  /** DEAD_LETTER_STATE_* */
  state?: string;
  nextAttemptAt?: string | number;
  lastHttpStatus?: number;
  lastError?: string;
  createTime?: string;
  updateTime?: string;
}

export interface AdminAuditLog {
  id: string;
  action: string;
  targetType: string;
  targetId?: string;
  namespace?: string;
  /** 当前无用户体系，恒为空 */
  operator?: string;
  sourceIp?: string;
  userAgent?: string;
  source?: string;
  /** 变更前/删除前的 JSON 快照 */
  snapshot?: string;
  /** OK | ERROR */
  result?: string;
  errorMessage?: string;
  createTime?: string;
}

export interface ListSchedulesResponse {
  page: PageResponse;
  schedules: Schedule[];
}

export interface ListScheduleDeadLettersResponse {
  page: PageResponse;
  deadLetters: ScheduleDeadLetter[];
}

export interface ListAdminAuditLogsResponse {
  page: PageResponse;
  logs: AdminAuditLog[];
}
