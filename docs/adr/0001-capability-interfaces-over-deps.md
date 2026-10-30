# ADR-0001：用能力接口取代 `Deps` 结构体装箱

- 状态：已交付
- 日期：2026-10（v1 归档期）
- 来源：从 [04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md) 拆分

## 背景与问题

一轮"去全局化"把 `global.DB` 换成 `Deps` 结构体传递，结果造出 32 个 `type Deps struct`（30 个含 `*gorm.DB`）、6 层传递链（`Infra → apideps.Deps → api.Deps → article_api.Deps → category.Deps → view_history.Deps`）、`api.New` 逐字段展开 15 个子 `Deps`。

它不是 DI，而是**看起来像 DI 的 Service Locator**。

## 决策

1. handler/service 构造函数只接收**窄接口**（每个 1–3 个方法），结构体 3–5 个字段；
2. 端口定义在**使用方**包内（`internal/article/app/port.go`），不是实现方；
3. `Deps` 结构体全部删除，archcheck 禁止新增；
4. 跨模块调用由**消费方**定义自己需要的接口（如 `comment` 定义 `ArticleGate`），不 import 别的模块；
5. 按模块渐进迁移，任何时刻代码都能编译。

## 理由与已否决方案

`Deps` 比裸 global 更糟——依赖"看起来可见"（有签名）但实际仍任取 DB，测试要构造 17 字段结构体更难，且**让人误以为重构已完成**；能力接口则真正受限（`DiggCounter` 只有一个 `Count`，调不到 `Delete`），拿不到 DB 连接想写 SQL 也写不了。

已否决的方案：

- **保留 `Deps` 只瘦身**：只要里面还有 `*gorm.DB`，任何包都能查任何表，属于"换个尺寸的创可贴"；
- **引入 wire / dig**：wire 让启动流程不可读、出错提示晦涩；dig 把编译期错误推到启动期；本项目 12 个模块，手写装配约 150 行足够。

## 代价 / 不做什么

- 构造函数参数变长（4–5 个）；
- 接口数增加到约 50 个（12 模块 × 约 4 端口）；
- 领域实体与持久化模型分离，每模块约 200 行 mapper 样板；
- 迁移期接口与 `Deps` 双轨并存。

## 当前生效约束

1. 禁止新增 `type Deps struct`；
2. `*gorm.DB` 只能出现在 `*/adapter/` 与 `platform/mysql/`，其余层出现数为 0；
3. 端口接口定义在使用方包内，实现方不得反向定义；
4. 模块之间不得互相 import，跨模块依赖一律通过消费方窄接口 + 合成根连线。

## 相关代码与文档

- 代码：[article/app/port.go](../../backend/internal/article/app/port.go)（端口定义在使用方包内）
- 代码：[tools/archcheck/rules.go](../../backend/tools/archcheck/rules.go)（禁止新增 `Deps`、`*gorm.DB` 只允许落在 adapter/platform 的守卫）
- 文档：[架构总纲 · 端口与能力接口](../architecture/overview.md)
- 原始记录：[04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md)
