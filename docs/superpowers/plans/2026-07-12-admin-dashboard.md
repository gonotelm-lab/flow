# Flow Admin Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a modern Linear-inspired admin dashboard in `web/` that manages Flow namespaces, tasks, and workers via the Admin HTTP API.

**Architecture:** React SPA with Vite dev proxy to the Flow admin gRPC-gateway HTTP server. TanStack Query for server state with 5s polling on tasks. App shell with icon sidebar; three list pages use inline accordion row expansion for details (no drawer, no sub-routes). `/` redirects to `/tasks`.

**Tech Stack:** React 18, Vite, TypeScript, Tailwind CSS v4, shadcn/ui, TanStack Query, React Router v7, lucide-react, sonner, cmdk

**Spec:** `docs/superpowers/specs/2026-07-12-admin-dashboard-design.md`

---

## Global Constraints

- All API paths under `/api/admin/v1/*` (see `api/admin/v1/rpc.proto`)
- Pagination: `page` (>=1), `page_size` (1–100), default page=1 page_size=20
- `bytes` proto fields arrive as base64 strings in JSON — decode for display
- Task states map: `INITED`=灰, `RUNNING`=琥珀, `DONE`=绿, `FAILED`=红, `CANCELLED`=灰
- Only one table row expanded at a time (accordion)
- Delete requires Dialog confirmation; Cancel only for INITED/RUNNING
- Theme: system default + manual toggle, both dark/light fully styled
- Vite proxy target configurable; Settings page stores base URL in localStorage

---

## File Structure

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `web/package.json` | Dependencies and scripts |
| Create | `web/vite.config.ts` | Dev server + API proxy |
| Create | `web/tsconfig.json` | TS config with path aliases |
| Create | `web/index.html` | HTML entry |
| Create | `web/components.json` | shadcn config |
| Create | `web/src/main.tsx` | React root + providers |
| Create | `web/src/App.tsx` | Router + redirect `/` → `/tasks` |
| Create | `web/src/index.css` | Tailwind + OKLCH theme variables |
| Create | `web/src/lib/utils.ts` | `cn()` helper |
| Create | `web/src/lib/format.ts` | Time, bytes, state formatters |
| Create | `web/src/lib/constants.ts` | TaskState labels/colors |
| Create | `web/src/lib/settings.ts` | localStorage API base URL |
| Create | `web/src/api/types.ts` | TS interfaces matching proto JSON |
| Create | `web/src/api/client.ts` | fetch wrapper |
| Create | `web/src/api/namespaces.ts` | Namespace API calls |
| Create | `web/src/api/tasks.ts` | Task API calls |
| Create | `web/src/api/workers.ts` | Worker API calls |
| Create | `web/src/components/layout/app-shell.tsx` | Shell wrapper |
| Create | `web/src/components/layout/sidebar.tsx` | Icon nav |
| Create | `web/src/components/layout/topbar.tsx` | Title + theme + ⌘K trigger |
| Create | `web/src/components/domain/*` | Tables + expand panels |
| Create | `web/src/pages/*.tsx` | Four pages |
| Create | `web/src/hooks/*.ts` | TanStack Query hooks |
| Create | `web/src/components/ui/*` | shadcn primitives |
| Test | `web/src/lib/format.test.ts` | Formatter unit tests |

---

### Task 1: Scaffold Vite + React + TypeScript

**Files:**
- Create: `web/package.json`
- Create: `web/vite.config.ts`
- Create: `web/tsconfig.json`
- Create: `web/tsconfig.app.json`
- Create: `web/tsconfig.node.json`
- Create: `web/index.html`
- Create: `web/src/main.tsx`
- Create: `web/src/App.tsx`
- Create: `web/src/vite-env.d.ts`

- [ ] **Step 1: Create `web/package.json`**

```json
{
  "name": "flow-admin",
  "private": true,
  "version": "0.0.1",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview",
    "test": "vitest run"
  },
  "dependencies": {
    "@tanstack/react-query": "^5.62.0",
    "class-variance-authority": "^0.7.1",
    "clsx": "^2.1.1",
    "cmdk": "^1.0.4",
    "lucide-react": "^0.468.0",
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "react-router-dom": "^7.1.0",
    "sonner": "^1.7.1",
    "tailwind-merge": "^2.6.0"
  },
  "devDependencies": {
    "@tailwindcss/vite": "^4.0.0",
    "@types/react": "^18.3.12",
    "@types/react-dom": "^18.3.1",
    "@vitejs/plugin-react": "^4.3.4",
    "tailwindcss": "^4.0.0",
    "typescript": "~5.6.2",
    "vite": "^6.0.3",
    "vitest": "^2.1.8"
  }
}
```

- [ ] **Step 2: Create `web/vite.config.ts`**

```typescript
import path from "node:path";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/api/admin": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});
```

- [ ] **Step 3: Create `web/tsconfig.json`, `tsconfig.app.json`, `tsconfig.node.json`**

`tsconfig.json`:
```json
{
  "files": [],
  "references": [
    { "path": "./tsconfig.app.json" },
    { "path": "./tsconfig.node.json" }
  ]
}
```

`tsconfig.app.json`:
```json
{
  "compilerOptions": {
    "tsBuildInfoFile": "./node_modules/.tmp/tsconfig.app.tsbuildinfo",
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "isolatedModules": true,
    "moduleDetection": "force",
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "noUncheckedSideEffectImports": true,
    "baseUrl": ".",
    "paths": { "@/*": ["./src/*"] }
  },
  "include": ["src"]
}
```

`tsconfig.node.json`:
```json
{
  "compilerOptions": {
    "tsBuildInfoFile": "./node_modules/.tmp/tsconfig.node.tsbuildinfo",
    "target": "ES2022",
    "lib": ["ES2023"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "isolatedModules": true,
    "moduleDetection": "force",
    "noEmit": true,
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "noUncheckedSideEffectImports": true
  },
  "include": ["vite.config.ts"]
}
```

- [ ] **Step 4: Create entry files**

`web/index.html`:
```html
<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Flow Admin</title>
    <link rel="preconnect" href="https://fonts.googleapis.com" />
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet" />
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

`web/src/vite-env.d.ts`:
```typescript
/// <reference types="vite/client" />
```

`web/src/main.tsx`:
```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";
import { Toaster } from "sonner";
import App from "./App";
import "./index.css";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, staleTime: 2000 },
  },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
        <Toaster richColors position="bottom-right" />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
```

`web/src/App.tsx` (placeholder routes):
```tsx
import { Navigate, Route, Routes } from "react-router-dom";

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/tasks" replace />} />
      <Route path="/tasks" element={<div>Tasks</div>} />
      <Route path="/workers" element={<div>Workers</div>} />
      <Route path="/namespaces" element={<div>Namespaces</div>} />
      <Route path="/settings" element={<div>Settings</div>} />
    </Routes>
  );
}
```

- [ ] **Step 5: Install and verify dev server**

Run:
```bash
cd web && npm install && npm run dev
```
Expected: Vite starts on `http://localhost:5173`, `/` redirects to `/tasks` showing "Tasks"

---

### Task 2: Theme + shadcn/ui Foundation

**Files:**
- Create: `web/src/index.css`
- Create: `web/components.json`
- Create: `web/src/lib/utils.ts`
- Create: `web/src/components/ui/button.tsx`
- Create: `web/src/components/ui/badge.tsx`
- Create: `web/src/components/ui/table.tsx`
- Create: `web/src/components/ui/tabs.tsx`
- Create: `web/src/components/ui/input.tsx`
- Create: `web/src/components/ui/select.tsx`
- Create: `web/src/components/ui/dialog.tsx`
- Create: `web/src/components/ui/skeleton.tsx`
- Create: `web/src/components/ui/dropdown-menu.tsx`
- Create: `web/src/components/theme-provider.tsx`

- [ ] **Step 1: Create `web/components.json`**

```json
{
  "$schema": "https://ui.shadcn.com/schema.json",
  "style": "new-york",
  "rsc": false,
  "tsx": true,
  "tailwind": {
    "config": "",
    "css": "src/index.css",
    "baseColor": "zinc",
    "cssVariables": true,
    "prefix": ""
  },
  "aliases": {
    "components": "@/components",
    "utils": "@/lib/utils",
    "ui": "@/components/ui",
    "lib": "@/lib",
    "hooks": "@/hooks"
  },
  "iconLibrary": "lucide"
}
```

- [ ] **Step 2: Create `web/src/index.css` with OKLCH theme tokens from spec §3.1**

Use shadcn Tailwind v4 CSS variable pattern. Dark as default in `.dark` class; light in `:root`. Key tokens:
- `--background`, `--foreground`, `--primary`, `--muted`, `--destructive`, `--border`, `--sidebar`
- Font: `--font-sans: "Inter", system-ui`, `--font-mono: "JetBrains Mono", monospace`
- Radius: `--radius: 0.375rem` (6px)

- [ ] **Step 3: Create `web/src/lib/utils.ts`**

```typescript
import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
```

- [ ] **Step 4: Add shadcn components**

Run from `web/`:
```bash
npx shadcn@latest add button badge table tabs input select dialog skeleton dropdown-menu
```

Accept defaults. Components land in `web/src/components/ui/`.

- [ ] **Step 5: Create `web/src/components/theme-provider.tsx`**

Implement theme context: read `prefers-color-scheme` on mount, persist manual override to `localStorage` key `flow-theme`, apply `.dark` class on `<html>`.

- [ ] **Step 6: Wrap App in ThemeProvider in `main.tsx`**

- [ ] **Step 7: Verify**

Run `npm run dev`, toggle theme — background switches between dark `oklch(0.13...)` and light `oklch(0.98...)`.

---

### Task 3: Format Utilities + API Client

**Files:**
- Create: `web/src/lib/constants.ts`
- Create: `web/src/lib/format.ts`
- Create: `web/src/lib/format.test.ts`
- Create: `web/src/lib/settings.ts`
- Create: `web/src/api/types.ts`
- Create: `web/src/api/client.ts`
- Create: `web/src/api/namespaces.ts`
- Create: `web/src/api/tasks.ts`
- Create: `web/src/api/workers.ts`

- [ ] **Step 1: Write failing tests in `web/src/lib/format.test.ts`**

```typescript
import { describe, expect, it } from "vitest";
import { formatRelativeTime, decodeBytes, formatTaskState } from "./format";

describe("formatRelativeTime", () => {
  it("returns seconds ago for recent timestamps", () => {
    const now = Date.now();
    expect(formatRelativeTime(new Date(now - 5000).toISOString())).toMatch(/ago|秒/);
  });
});

describe("decodeBytes", () => {
  it("parses base64 JSON", () => {
    const json = Buffer.from('{"key":"val"}').toString("base64");
    expect(decodeBytes(json)).toBe('{\n  "key": "val"\n}');
  });
});

describe("formatTaskState", () => {
  it("maps RUNNING to label", () => {
    expect(formatTaskState("RUNNING").label).toBe("Running");
  });
});
```

- [ ] **Step 2: Run test — expect FAIL**

```bash
cd web && npm run test
```

- [ ] **Step 3: Implement `web/src/lib/constants.ts`**

```typescript
export const TASK_STATES = [
  "TASK_STATE_UNSPECIFIED",
  "INITED",
  "RUNNING",
  "DONE",
  "FAILED",
  "CANCELLED",
] as const;

export type TaskState = (typeof TASK_STATES)[number];

export const TASK_STATE_META: Record<string, { label: string; color: string }> = {
  INITED: { label: "Inited", color: "bg-zinc-500" },
  RUNNING: { label: "Running", color: "bg-amber-500" },
  DONE: { label: "Done", color: "bg-green-500" },
  FAILED: { label: "Failed", color: "bg-red-500" },
  CANCELLED: { label: "Cancelled", color: "bg-zinc-400" },
};
```

- [ ] **Step 4: Implement `web/src/lib/format.ts`**

```typescript
import { TASK_STATE_META } from "./constants";

export function formatRelativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const sec = Math.floor(diff / 1000);
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  return `${Math.floor(hr / 24)}d ago`;
}

export function decodeBytes(b64?: string): string {
  if (!b64) return "—";
  try {
    const raw = atob(b64);
    const parsed = JSON.parse(raw);
    return JSON.stringify(parsed, null, 2);
  } catch {
    return b64;
  }
}

export function formatTaskState(state: string) {
  return TASK_STATE_META[state] ?? { label: state, color: "bg-zinc-500" };
}

export function truncateId(id: string, len = 8): string {
  return id.length > len * 2 ? `${id.slice(0, len)}…${id.slice(-4)}` : id;
}
```

- [ ] **Step 5: Implement `web/src/lib/settings.ts`**

```typescript
const KEY = "flow-api-base-url";
const DEFAULT = "";

export function getApiBaseUrl(): string {
  return localStorage.getItem(KEY) ?? DEFAULT;
}

export function setApiBaseUrl(url: string) {
  localStorage.setItem(KEY, url);
}
```

- [ ] **Step 6: Implement `web/src/api/types.ts`**

Define interfaces: `PageRequest`, `PageResponse`, `Namespace`, `Task`, `TaskEvent`, `Worker`, list response wrappers. Match proto JSON field names (camelCase from grpc-gateway).

- [ ] **Step 7: Implement `web/src/api/client.ts`**

```typescript
import { getApiBaseUrl } from "@/lib/settings";

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const base = getApiBaseUrl();
  const url = `${base}${path}`;
  const res = await fetch(url, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  if (!res.ok) {
    const text = await res.text();
    throw new ApiError(res.status, text || res.statusText);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}
```

- [ ] **Step 8: Implement API modules**

`namespaces.ts`: `listNamespaces(page, pageSize)`, `getNamespace(name)`, `createNamespace(body)`, `updateNamespace(name, body)`

`tasks.ts`: `listTasks(params)`, `getTask(id)`, `cancelTask(id)`, `deleteTask(id)`, `listTaskEvents(taskId, page, pageSize)`

`workers.ts`: `listWorkers(params)`, `getWorker(id)`

Query string builder for optional filters (namespace, task_type, state).

- [ ] **Step 9: Run tests — expect PASS**

```bash
cd web && npm run test
```

---

### Task 4: App Shell + Router

**Files:**
- Create: `web/src/components/layout/app-shell.tsx`
- Create: `web/src/components/layout/sidebar.tsx`
- Create: `web/src/components/layout/topbar.tsx`
- Modify: `web/src/App.tsx`

- [ ] **Step 1: Create `sidebar.tsx`**

56px icon sidebar with NavLink to `/tasks`, `/workers`, `/namespaces`, `/settings`. Icons: `ListTodo`, `Cpu`, `Boxes`, `Settings` from lucide-react. Active state: `bg-primary/10 text-primary`.

- [ ] **Step 2: Create `topbar.tsx`**

Props: `title: string`. Right side: theme toggle button (Sun/Moon), ⌘K button placeholder.

- [ ] **Step 3: Create `app-shell.tsx`**

```tsx
export function AppShell({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="flex h-screen bg-background text-foreground">
      <Sidebar />
      <div className="flex flex-1 flex-col overflow-hidden">
        <Topbar title={title} />
        <main className="flex-1 overflow-auto p-6">{children}</main>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Update `App.tsx` to wrap routes in AppShell with correct titles**

- [ ] **Step 5: Verify navigation**

Click sidebar icons — URL changes, title updates, no full page reload.

---

### Task 5: Shared Table + Inline Expand Pattern

**Files:**
- Create: `web/src/components/domain/expandable-row.tsx`
- Create: `web/src/components/domain/pagination.tsx`
- Create: `web/src/components/domain/status-dot.tsx`
- Create: `web/src/components/domain/empty-state.tsx`

- [ ] **Step 1: Create `status-dot.tsx`**

Colored circle + optional pulse animation for RUNNING.

- [ ] **Step 2: Create `expandable-row.tsx`**

```tsx
type ExpandableRowProps = {
  isOpen: boolean;
  onToggle: () => void;
  colSpan: number;
  summary: React.ReactNode;
  detail: React.ReactNode;
};
```

Renders `<tr>` for summary row (click toggles), then `{isOpen && <tr><td colSpan={...}><div className="bg-muted/50 border-y px-4 py-4">{detail}</div></td></tr>}`.

Accordion: parent passes `expandedId` state; only one open.

- [ ] **Step 3: Create `pagination.tsx`**

Props: `page`, `pageSize`, `totalCount`, `onPageChange`. Prev/Next + "Page X of Y".

- [ ] **Step 4: Create `empty-state.tsx`**

Icon + message + optional action button.

---

### Task 6: Tasks Page

**Files:**
- Create: `web/src/hooks/use-tasks.ts`
- Create: `web/src/components/domain/task-table.tsx`
- Create: `web/src/components/domain/task-expand-panel.tsx`
- Create: `web/src/pages/tasks-page.tsx`
- Modify: `web/src/App.tsx`

- [ ] **Step 1: Create `use-tasks.ts`**

```typescript
export function useTasks(filters: TaskFilters) {
  return useQuery({
    queryKey: ["tasks", filters],
    queryFn: () => listTasks(filters),
    refetchInterval: 5000,
  });
}

export function useTask(id: string | null) {
  return useQuery({
    queryKey: ["task", id],
    queryFn: () => getTask(id!),
    enabled: !!id,
    refetchInterval: 5000,
  });
}
```

Plus `useCancelTask`, `useDeleteTask`, `useTaskEvents` mutations/queries.

- [ ] **Step 2: Create `task-expand-panel.tsx`**

Sections: metadata grid, payload/result/error in `<pre className="font-mono text-xs">`, events list with pagination.

Cancel/Delete buttons in header. Delete opens Dialog.

- [ ] **Step 3: Create `task-table.tsx`**

Columns per spec §2.1. Status Tabs at top. Namespace Select + task_type Input filters.

Uses ExpandableRow. `expandedId` state in component.

- [ ] **Step 4: Create `tasks-page.tsx`**

```tsx
export function TasksPage() {
  return (
    <AppShell title="Tasks">
      <TaskTable />
    </AppShell>
  );
}
```

- [ ] **Step 5: Wire route in App.tsx**

- [ ] **Step 6: Manual test against running Flow server**

Start Flow admin server, run `npm run dev`, verify task list loads, row expands, cancel/delete work.

---

### Task 7: Workers Page

**Files:**
- Create: `web/src/hooks/use-workers.ts`
- Create: `web/src/components/domain/worker-table.tsx`
- Create: `web/src/components/domain/worker-expand-panel.tsx`
- Create: `web/src/pages/workers-page.tsx`

- [ ] **Step 1: Create hooks and API integration**

`useWorkers(filters)` with standard staleTime, no auto-poll (or 30s optional).

- [ ] **Step 2: Create worker-table + worker-expand-panel**

Heartbeat column: `formatRelativeTime`, red text if >30s stale.

Expand panel: metadata + stats (success/total dealt).

- [ ] **Step 3: Create workers-page and wire route**

- [ ] **Step 4: Manual test**

---

### Task 8: Namespaces Page

**Files:**
- Create: `web/src/hooks/use-namespaces.ts`
- Create: `web/src/components/domain/namespace-table.tsx`
- Create: `web/src/components/domain/namespace-expand-panel.tsx`
- Create: `web/src/components/domain/namespace-create-form.tsx`
- Create: `web/src/pages/namespaces-page.tsx`

- [ ] **Step 1: Create hooks**

`useNamespaces`, `useCreateNamespace`, `useUpdateNamespace` mutations with query invalidation.

- [ ] **Step 2: Create namespace-create-form.tsx**

Inline form row at table top (toggle via "Create Namespace" button). Fields: name, description, creator. On success show full `api_key` once with copy button.

- [ ] **Step 3: Create namespace-expand-panel.tsx**

Edit description/creator form. Show `api_key_preview`. Save calls `updateNamespace`.

- [ ] **Step 4: Create namespaces-page and wire route**

- [ ] **Step 5: Manual test create + edit flow**

---

### Task 9: Settings Page

**Files:**
- Create: `web/src/pages/settings-page.tsx`

- [ ] **Step 1: Create settings form**

Input for API base URL (empty = use Vite proxy). "Test Connection" button calls `GET /api/admin/v1/namespaces?page=1&page_size=1`. Toast success/failure.

- [ ] **Step 2: Wire route**

- [ ] **Step 3: Verify**

Change base URL, test connection, confirm tasks still load.

---

### Task 10: Command Palette (⌘K)

**Files:**
- Create: `web/src/components/domain/command-menu.tsx`
- Modify: `web/src/components/layout/topbar.tsx`

- [ ] **Step 1: Add shadcn command component**

```bash
cd web && npx shadcn@latest add command
```

- [ ] **Step 2: Create command-menu.tsx**

Uses `cmdk`. Global keydown listener for `Meta+K` / `Ctrl+K`. Items:
- Navigate: Tasks, Workers, Namespaces, Settings
- Quick filter: "Failed tasks" → navigate /tasks with failed filter

- [ ] **Step 3: Integrate in AppShell or Topbar**

- [ ] **Step 4: Verify ⌘K opens palette, navigation works**

---

### Task 11: Polish + Build Verification

**Files:**
- Modify: various components for loading skeletons, error states
- Create: `web/public/favicon.svg` (simple "flow" mark)

- [ ] **Step 1: Add skeleton loading to all three tables**

6 skeleton rows while `isLoading`.

- [ ] **Step 2: Add error boundaries / error toast on query failure**

- [ ] **Step 3: Add `prefers-reduced-motion` — disable expand animation**

- [ ] **Step 4: Production build**

```bash
cd web && npm run build
```
Expected: `dist/` generated without errors.

- [ ] **Step 5: Configure live mode (optional)**

Write `.impeccable/live/config.json` for `/impeccable live` with Vite HTML entry.

---

## Self-Review

| Spec Requirement | Task |
|-----------------|------|
| `/` → `/tasks` redirect | Task 1, 4 |
| Icon sidebar layout | Task 4 |
| Tasks primary with status tabs + filters | Task 6 |
| Inline accordion expand (no drawer) | Task 5, 6, 7, 8 |
| 5s task polling | Task 6 |
| All 11 API endpoints used | Task 3, 6, 7, 8 |
| Dark/light theme toggle | Task 2 |
| Settings API base URL | Task 9 |
| ⌘K command palette | Task 10 |
| Delete confirmation Dialog | Task 6 |
| Namespace create shows api_key once | Task 8 |

No TBD placeholders found.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-12-admin-dashboard.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks
2. **Inline Execution** — execute tasks in this session with checkpoints

Which approach?
