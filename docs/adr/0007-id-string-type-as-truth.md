# ADR-0007：ID 类型统一为 string 底层（类型即真相）

- 状态：已交付
- 日期：2026-10（v1 归档期）
- 来源：从 [04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md) 拆分

## 背景与问题

后端 `type ID uint64` 却在 `MarshalJSON` 里输出字符串，swag 只读 Go 类型声明（`uint64`），导致契约里 100 处 ID 印成 `number` 而实际传输是 `"372842048379686912"`（18 位，超 JS 安全整数 2^53-1）。

只能靠 `fix-openapi-id-types.mjs` 做 138 处 integer→string 后处理，且全仓有 **10 套**并存的 ID 类型。

## 决策

1. `ctype.ID` 底层改 `string`，提供 `Uint64()` / `IsZero()` / `Uint64OrZero()` / `ParseID()` / `Value()` / `Scan()`（入库出库仍转 int64，**零 DDL**）；
2. 其余 9 个 domain ID 类型收敛为 `ctype.ID` 的别名或同构 string 类型；
3. 89 处裸 `uint64` ID 字段收编进 `ctype.ID`；
4. 237 处非 ID 整型（`Page` / `Limit` / `DiggCount` / `Size` / `Score`）**一个都不动**；
5. 追加：swag 换 v2 + `--v3.1`，流水线 4 步 → 3 步 → **2 步**。

## 理由与已否决方案

类型即真相——隐式转换消失后"Go 类型"与"JSON 形态"永远是同一个东西，swag 不可能印错，`.swaggo` 与后处理脚本可删。

已否决的方案：

- **`.swaggo` 全局覆盖**：只买"这次对了"，管不到裸 `uint64`；
- **逐字段 `swaggertype:"string"`**：那些字段真的发数字，标注等于让契约撒谎；
- **换 huma / go-fuego**：123 个接口全部重来。

## 代价 / 不做什么

- 实际改动约 490 处（预估 240），3–4 轮才收敛；
- **6 处 ID 大小比较会静默变语义**（字符串下 `"10" < "9"` 成立），必须改成显式 `a.ID.Uint64() < b.ID.Uint64()`；
- 26 处按理由豁免（log 6 / testkit schema 17 / `ServerID` 4 / `initializedID` 1）；
- golden 需重生成。

## 当前生效约束

1. archcheck **R12 `id-must-be-ctype`**（`backend/tools/archcheck/rules_id.go`）：字段名以 `ID` / `IDs` / `Id` / `Ids` 结尾且类型为原生整型（含 `*` / `[]`）→ 违规，**基线 0**；
2. 验证口径必须与门禁一致（`go vet ./...` 不编译 `//go:build integration` 文件）；
3. `docs/` 只保留 `swagger.json` 一个产物，`docs.go` / `swagger.yaml` 已删。

## 相关代码与文档

- 代码：[kernel/ctype/id.go](../../backend/internal/kernel/ctype/id.go)（`ctype.ID` 的 string 底层与转换方法）
- 代码：[tools/archcheck/rules_id.go](../../backend/tools/archcheck/rules_id.go)（R12 `id-must-be-ctype`）
- 文档：[契约 · ADR-007：ID 类型底层统一为 string](../architecture/contracts.md)（统一策略、四类豁免、唯一不能靠编译器发现的风险）
- 原始记录：[04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md)
