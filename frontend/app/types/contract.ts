/**
 * OpenAPI 契约类型入口 + 视图类型工具。
 *
 * 链路：backend/docs/swagger.json（swag v2 从代码注解直出 OpenAPI 3.1）
 *   → `pnpm openapi:types`（openapi-typescript 一步生成）
 *   → ./openapi.d.ts（自动生成，勿手改）
 *
 * # 现在的关系（2026-10-26 重构后）
 *
 * 后端 123 个 `@Success` 注解已全部改为引用真实 Go 类型（不再是手抄的 `object{...}`），
 * 所以契约的响应形状是**从代码推导**的。相应地前端 `api.ts` 里的核心类型也改成
 * 从契约派生，`contract-guard.ts`（比字段名的对账单）已删除——不再需要，
 * 字段名和类型现在都在编译期校验。
 *
 * `api.ts` 里仍然手写的只剩两类：
 *   - 纯 UI 形状（`ApiEnvelope`、`OptionItem`、`ChatSocketEnvelope` 等），本就不对应接口
 *   - 列表包装 `{list, count}`（契约里是超长的泛型实例名，手写更可读）
 *
 * # 三层工具
 *
 * - `Schema<K>`       —— 契约原样（全字段可选），用于请求体
 * - `RequiredSchema<K>` —— 递归去掉 `?` 和 `| undefined`，用于响应体
 * - `Nullable<T, K>`  —— 把指定字段放宽为可空（swag 不表达 null，只能前端声明）
 * - `OptionalFields<T, K>` —— 把指定字段放回可选（后端确实可能不返回）
 */
import type { components, paths } from "./openapi";

/** 取某个 schema 的类型，如 `Schema<"api_banner_api.BannerCreateRequest">`。 */
export type Schema<K extends keyof components["schemas"]> = components["schemas"][K];

/** 取某个操作的类型，如 `Operation<"/banners", "post">["responses"]`。 */
export type Operation<P extends keyof paths, M extends keyof paths[P]> = paths[P][M];

/**
 * 把契约 schema 变成「字段必填」的视图类型。
 *
 * 契约里所有字段都是可选的（后端响应 struct 不写 binding，swag 就没有 required 可读），
 * 直接拿来当业务类型会让每个字段都多出 `undefined`。但**响应实际总是带这些字段**，
 * 所以业务侧用 `Required<>` 恢复必填——这比手写 interface 更准：
 * 字段名和类型都来自契约，后端改名/改类型都会立刻编译报错。
 */
export type RequiredSchema<K extends keyof components["schemas"]> = DeepRequired<components["schemas"][K]>;

/**
 * 递归地去掉所有 `?` 和 `| undefined`。
 *
 * 只用 `Required` 不够：它只作用于顶层属性，嵌套对象的字段仍然是可选的
 * （`author?: { avatar?: string }` 里的 `avatar` 还是 `string | undefined`），
 * 结果业务侧照样到处判空。所以必须往下递归。
 *
 * 对数组要单独处理（`DeepRequired<T[]> = DeepRequired<T>[]`），
 * 否则 `keyof T[]` 会展开成数组的所有方法名。
 */
export type DeepRequired<T> = T extends (infer U)[]
  ? DeepRequired<U>[]
  : T extends object
    ? { [K in keyof T]-?: DeepRequired<Exclude<T[K], undefined>> }
    : Exclude<T, undefined>;

/**
 * 把指定字段放宽为可空。
 *
 * 为什么需要它：Go 里的 `*T` 字段会序列化成 `null`，但 swag **不表达空值**
 * （`nullable` / `type:[x,null]` 全仓 0 处，v2 也没这个能力）。
 * 所以"哪些字段真的是 null"只能在前端显式声明。
 *
 * 好处是字段名写错、契约里删掉该字段，都会编译报错——谎言的边界是可见的。
 */
export type Nullable<T, K extends keyof T> = Omit<T, K> & { [P in K]: T[P] | null };

/**
 * 把指定字段改回可选（允许对象里不出现）。
 *
 * `DeepRequired` 会把契约里所有字段变必填，但有些字段后端**确实可能不返回**
 * （比如 `deleted_at` 只在软删后才有）。这类字段用这个工具单独放回可选，
 * 而不是整体退回 `Required` 之外。
 */
export type OptionalFields<T, K extends keyof T> = Omit<T, K> & { [P in K]?: T[P] };
