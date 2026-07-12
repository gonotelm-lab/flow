# Product

## Register

product

## Platform

web

## Users

平台运维人员（Platform Ops）。他们日常需要监控 Flow 分布式任务队列的运行状态：查看任务是否失败或卡住、检查 Worker 心跳健康、管理 Namespace 和 API Key。使用场景是长时间盯屏的桌面浏览器，追求快速定位异常并执行操作（取消任务、删除任务、编辑 namespace）。

## Product Purpose

为 Flow 服务提供一套现代风格的管理后台，通过 Admin HTTP API 实现 namespace、task、worker 的可视化管理。成功标准是：运维人员打开后台后 10 秒内能发现并处理异常任务，无需借助 CLI 或数据库直连。

## Positioning

Flow 后台不是另一个 Grafana——它是为 Flow 任务模型量身定制的轻量运维面板，任务状态一目了然，操作路径最短。

## Brand Personality

精准、克制、高效。三个词：Clear · Fast · Trustworthy。界面应像 Linear 一样让人信任——每个状态色、每个按钮都语义明确，不装饰。

## Anti-references

- SaaS 奶油风：暖米色背景、大圆角卡片、渐变装饰
- 传统 Bootstrap Admin：蓝色顶栏、厚重边框、图标+卡片网格
- Grafana 克隆：过多图表、橙色系、运维面板堆砌感
- 每个 section 上方的 tracked uppercase eyebrow
- 首页 hero-metric 大数字模板

## Design Principles

1. **先选命名空间，再干活** — 首页（`/`）为命名空间选择页；选定后进入任务列表，状态筛选一步到位，不做仪表盘绕路。未选 namespace 时，`/tasks`、`/workers` 重定向回首页。
2. **列表即中心** — 三个模块各一个列表页。任务详情用 Dialog 展示（载荷、事件时间轴等信息密度高）；Worker 等轻量详情用行内手风琴展开。列表页本身不做页面跳转。
3. **状态色说话** — 用精准的语义色传达任务状态，不靠装饰性图标
4. **熟悉但不无聊** — 沿用 Linear/现代工具的标准模式（侧栏、表格、Dialog 详情、⌘K），不发明新 affordance
5. **双主题平等** — 深色和浅色都是一等公民，跟随系统且可手动切换

## Accessibility & Inclusion

无特殊 WCAG 等级要求。遵循 shadcn/ui 默认语义 HTML 和基本对比度。
