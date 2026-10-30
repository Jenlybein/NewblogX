# ADR-0003：前后端契约以代码生成为单一来源

- 状态：已交付
- 日期：2026-10（v1 归档期）
- 来源：从 [04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md) 拆分

## 背景与问题

`openapi/blogx.openapi.json` 是 14,462 行的手写文档，全仓引用数为 **0**；前端 1,005 行 interface 手写（`frontend/app/types/api.ts` 736 行 80 个 + admin 269 行 28 个）。

118 个后端 operation ↔ 108 个前端 interface 零机械校验，字段改名只在运行时暴露为 `undefined` 或 500。

## 决策

Go handler + struct + swag 注解 → `backend/docs/swagger.json` → 各前端 `openapi-typescript` 生成 `app/types/openapi.d.ts`。

落地节奏：

- S2 只做类型生成不动页面；
- S5 删除全部手写 API 类型；
- CI 校验"生成结果与提交一致"。

## 理由与已否决方案

漂移从"运行时 500"变成"编译期报错"，且让 openapi 从文档负债变成可做 Mock / SDK / 文档站的资产。

已否决的方案：

- **tygo 直接出 TS**：只覆盖 struct，无法表达路由、状态码、参数位置；
- **tRPC / Connect**：要改 Gin 路由层与前端调用方式，迁移成本远超收益。

## 代价 / 不做什么

- 118 个 operation 补注解是一次性工作量；
- 生成产物入库带来 PR diff 噪音；
- swag 对泛型响应支持有限。

**不追求消灭所有手写类型**——纯前端状态类型（`ThemeMode`、UI props）与后端未暴露的内部结构仍手写。

## 当前生效约束

1. **凡是跨网络传输的类型，都必须来自生成**；
2. 生成产物放两端 `app/types/openapi.d.ts`，与手写类型分文件；
3. 前端禁止从 `openapi.d.ts` 之外 import API 类型；
4. CI 三处一致性校验，防止有人手改产物。

## 相关代码与文档

- 代码：[backend/docs/swagger.json](../../backend/docs/swagger.json)（唯一的契约产物）
- 代码：[frontend/app/types/openapi.d.ts](../../frontend/app/types/openapi.d.ts)、[frontend_admin/app/types/openapi.d.ts](../../frontend_admin/app/types/openapi.d.ts)（生成产物）
- 文档：[契约：单一来源、生成流水线与类型真相](../architecture/contracts.md)（流水线与 swag 的坑）
- 原始记录：[04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md)
