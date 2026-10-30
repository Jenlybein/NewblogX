# ADR-0002：垂直模块切分与合成根

- 状态：已交付
- 日期：2026-10（v1 归档期）
- 来源：从 [04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md) 拆分

## 背景与问题

原先是水平切分（`api/` 170 包、`service/` 170 包、`repository/` 8 包、`models/` 48 文件），业务逻辑全压在 handler 上（`article_api` 6,012 行 vs `article_service` 99 行、`comment_service` 11 行），65 个 `enter.go` 共 1,327 行全是包装。

**"每层都均匀有内容"这个隐含假设已被证伪。**

## 决策

1. 采用垂直切分：每个业务模块自包含 `domain / app / adapter` 三段；
2. `adapter` 合并 http 与 store（不拆两个包），代码量用**文件粒度**控制：`http.go` < 500 行、`mysql.go` < 800 行；
3. 合成根唯一：`cmd/server/main.go` 是唯一允许 import 具体实现的地方，`main.go` 之外不得 `new` 具体实现；
4. 每个模块的 `adapter/wire.go` 提供一个 `wireXxx(infra)` 函数；无包级可变状态；
5. 跨模块协作三条规则：

| 需求 | 做法 | 禁止 |
| --- | --- | --- |
| 读别的模块数据 | 消费方定义窄接口（`ArticleGate`） | import 别的模块 domain |
| 被通知 | 发领域事件（outbox） | 直接调别的模块 service |
| 共享值对象 | 放 `internal/kernel` | 把业务实体放 kernel |

## 理由与已否决方案

改动局部化（一个功能在 `internal/article/` 内）、可按模块独立迁移上线、包数从 300+ 降到 36（12 模块 × 3）。http 与 store 合并是因为两者生命周期一致（都由合成根构造、都是"与外界打交道"），分开只增加 import 不增加约束。

已否决的方案：

- **保持水平切分补空**：强制制造空壳，`comment_service` 11 行就是榜样；
- **水平切分 + `internal/` 强制边界**：改善了症状没解决病因，5 个模块没有 service 会让边界规则形式化；
- **完整 DDD（限界上下文 + 防腐层 + 领域事件）**：83k 行项目上过度设计。

## 代价 / 不做什么

- 目录认知成本；
- 跨模块调用变啰嗦（要定义接口、在合成根接线）；
- `article` 数据被 `comment` / `search` / `ai` 使用时需各自定义只读接口；
- 迁移期旧 `api/ service/` 与新 `internal/` 并存。

## 当前生效约束

1. 依赖单向 `adapter → app → domain`，编译器可校验；
2. 模块间零 import；
3. `internal/kernel` 只放 ID、分页、错误，不得放业务实体；
4. 每个模块迁移前录 golden file，保证行为不变。

## 相关代码与文档

- 代码：[cmd/server/main.go](../../backend/cmd/server/main.go)（唯一合成根）
- 代码：[internal/article](../../backend/internal/article)（模块内 `domain / app / adapter` 三段布局的样例）
- 代码：[internal/kernel](../../backend/internal/kernel)（只放 ID、分页、错误等共享值对象）
- 文档：[架构总纲](../architecture/overview.md)（§2.2 合成根 / §2.4 模块划分 / §2.5 分层与依赖方向）
- 原始记录：[04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md)
