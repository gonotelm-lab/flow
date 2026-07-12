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

Flow Admin 是面向 Platform Ops 的轻量运维面板。视觉语言借鉴 Linear：冷灰 OKLCH 中性色（hue 264）、Geist 无衬线、表格为中心的信息架构。状态通过语义色（success / warning / destructive）和 StatusDot 双编码传达，不做装饰性图表或仪表盘绕路。

## Colors

- **策略**：Restrained — 冷灰中性底 + 语义色点缀
- **浅色背景**：`oklch(0.99 0.002 264)` — 非奶油暖色
- **深色背景**：`oklch(0.18 0.006 264)`
- **语义色**：success（绿 155°）、warning（黄 75°）、destructive（红 25°）、info（蓝 250°）

## Typography

- **主字体**：Geist Variable — 单一无衬线家族承载全部 UI
- **层级**：固定 rem 比例（product register），h1 在 Topbar 为 `text-sm font-semibold`
- **正文**：`text-sm`，辅助文案 `text-muted-foreground`

## Elevation

- 侧栏：`border-r` 分隔，无阴影
- Topbar：`backdrop-blur-md` + `bg-background/80` 粘性顶栏
- 卡片（page-panel）：`border` + `shadow-sm`
- Dialog：`max-h-[85vh]` 内容区滚动；任务详情支持 `?task=` URL 深链

## Components

- **Sidebar**：52px/14px 可折叠，nav-active-indicator 左侧竖线，任务/工作节点异常 badge
- **Tables**：colgroup 固定列宽，Skeleton 加载态，任务 ID 可复制
- **StatusDot**：2px 圆点，RUNNING 带 ping 动画
- **CommandMenu**：cmdk Dialog，⌘K 快捷键，含快捷筛选与快捷键说明

## Do's and Don'ts

**Do**
- 用状态色和 dot 传达任务/心跳健康
- 保持列表页信息密度，详情渐进披露
- 异常计数常驻 chrome 层（sidebar badge）

**Don't**
- 暖米色奶油背景或大圆角卡片网格
- 每 section 上方 tracked uppercase eyebrow
- hero-metric 大数字模板
- Grafana 式图表堆砌
