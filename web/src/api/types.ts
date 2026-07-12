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
