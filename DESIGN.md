---
name: Flow Admin
description: 精准克制的 Flow 任务队列运维面板
colors:
  background: oklch(0.99 0.002 264)
  foreground: oklch(0.18 0.01 264)
  primary: oklch(0.22 0.015 264)
  muted: oklch(0.96 0.004 264)
  muted-foreground: oklch(0.48 0.01 264)
  destructive: oklch(0.52 0.19 25)
  success: oklch(0.52 0.14 155)
  warning: oklch(0.62 0.14 75)
  info: oklch(0.55 0.1 250)
  sidebar: oklch(0.985 0.003 264)
  border: oklch(0.91 0.005 264)
  dark-background: oklch(0.18 0.006 264)
  dark-sidebar: oklch(0.16 0.006 264)
  dark-card: oklch(0.21 0.006 264)
  dark-border: oklch(0.29 0.006 264)
typography:
  sans:
    fontFamily: "Geist Variable, Geist, system-ui, sans-serif"
    fontWeight: 400
rounded:
  sm: 0.375rem
  md: 0.5rem
  lg: 0.625rem
spacing:
  page: 1.5rem
  section: 1rem
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.background}"
    rounded: "{rounded.md}"
    padding: "0.5rem 1rem"
  badge-destructive:
    backgroundColor: "{colors.destructive}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.sm}"
    padding: "0.125rem 0.5rem"
---

## Overview

Flow Admin 是面向 Platform Ops 的轻量运维面板。视觉语言借鉴 Linear：冷灰 OKLCH 中性色（hue 264）、Geist 无衬线单一字体家族、表格为中心的信息架构。状态通过语义色（success / warning / destructive）和 StatusDot 双编码传达，不做装饰性图表或仪表盘绕路。

## Colors

- **策略**：Restrained — 冷灰中性底 + 语义色点缀
- **浅色背景**：`oklch(0.99 0.002 264)` — 非奶油暖色
- **深色背景**：`oklch(0.18 0.006 264)` — 较早期版本提亮，减轻盯屏压迫感
- **深色层级**：sidebar `0.16`、card `0.21`、border `0.29`
- **语义色**：success（绿 155°）、warning（黄 75°）、destructive（红 25°）、info（蓝 250°）

## Typography

- **唯一字体**：Geist Variable — 表格 ID、API Key、载荷等全部使用 sans，不用等宽字体
- **层级**：Topbar 标题 `text-sm font-semibold`；表格正文 `text-sm`；辅助 `text-muted-foreground`
- **数字**：ID、计数等需要对齐处用 `tabular-nums`，非 `font-mono`

## Layout

- **App shell**：侧栏 + Topbar + `max-w-7xl` 主内容区（`p-6`）
- **列表页工具栏**：左侧状态 Tab（任务页）/ 空白；右侧筛选 Input + outline Search icon 按钮
- **表格操作列**：`th/td` 用 `p-0`，内层 `flex h-11 items-center px-3` 与表头对齐；取消/删除 `gap-3`，取消固定 `w-8` 槽位保证删除纵向对齐

## Elevation

- 侧栏：`border-r` 分隔，无阴影；展开 `w-52`，折叠 `w-14`
- Topbar：`backdrop-blur-md` + `bg-background/80` 粘性顶栏；含 NamespaceSwitcher
- 卡片（page-panel）：`border` + `shadow-sm`，命名空间创建 API Key 提示等场景使用
- Dialog：任务详情 `max-h-[85vh]` 内容区滚动；URL 参数 `?task=` 与 Dialog 状态同步

## Components

- **Sidebar**：header 内 logo + 折叠 icon（无文案）；nav-active-indicator 左侧竖线；任务/工作节点 destructive badge（折叠态为圆点）
- **Tables**：`table-auto` + colgroup；ID 列 `minWidth: 20rem` + `whitespace-nowrap`；Skeleton 加载态
- **StatusDot**：2px 圆点，RUNNING / 异常心跳带 pulse
- **CommandMenu**：cmdk Dialog；分组为「导航」「快捷筛选」「快捷键」；group heading 无 `tracking-wide`
- **NamespaceSwitcher**：Topbar outline 按钮，`text-sm` sans

## Interaction Patterns

| 模式 | 行为 |
|------|------|
| 任务筛选 | draft 输入 + debounce 自动查询；Search / Enter 立即应用 |
| 任务详情 | 点击行打开 Dialog；URL 写入 `?task=id`；改筛选时关闭详情 |
| 任务取消 | INITED/RUNNING 可点；其他状态按钮 `disabled opacity-40` 仍占位 |
| 数据刷新 | React Query `refetchInterval`（任务 5s、worker 异常计数 30s）；UI 不展示「刚刚更新」文案 |

## Do's and Don'ts

**Do**
- 用状态色和 dot 传达任务/心跳健康
- 保持列表页信息密度，详情渐进披露
- 异常计数常驻 chrome 层（sidebar badge）
- 操作列按钮始终占位，禁用态用置灰而非隐藏

**Don't**
- 暖米色奶油背景或大圆角卡片网格
- 每 section 上方 tracked uppercase eyebrow
- hero-metric 大数字模板
- Grafana 式图表堆砌
- 页面内重复铺告警 strip（与侧栏 badge 职能重叠）
- JetBrains Mono 或第二字体家族
