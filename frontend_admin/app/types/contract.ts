/**
 * OpenAPI 契约类型入口 + 视图类型工具。
 *
 * 链路：backend/docs/swagger.json（swag v2 从代码注解直出 OpenAPI 3.1）
 *   → `pnpm openapi:types`（openapi-typescript 一步生成）
 *   → ./openapi.d.ts（自动生成，勿手改）
 *
 * # 与 api.ts 的关系（2026-10-26 起）
 *
 * 后端 123 个 `@Success` 注解已全部改为引用真实 Go 类型，响应形状是**从代码推导**的。
 * 所以 `api.ts` 里能对上契约的类型一律改成从契约派生（字段名和类型都在编译期校验），
 * 只保留契约里没有的东西：纯 UI 形状、本地泛型、以及契约与后端不一致的少数几个。
 *
 * # 三层工具
 *
 * - `Schema<K>`         —— 契约原样（全字段可选），用于请求体
 * - `RequiredSchema<K>` —— 递归去掉 `?` 和 `| undefined`，用于响应体
 * - `Nullable<T, K>`    —— 把指定字段放宽为可空（swag 不表达 null，只能前端声明）
 * - `OptionalFields<T, K>` —— 把指定字段放回可选（后端确实可能不返回）
 */
import type { components, paths } from "./openapi";

/** 取某个 schema 的类型，如 `Schema<"internal_site_adapter_http.BannerItem">`。 */
export type Schema<K extends keyof components["schemas"]> = components["schemas"][K];

/** 取某个操作的类型，如 `Operation<"/banners", "post">["responses"]`。 */
export type Operation<P extends keyof paths, M extends keyof paths[P]> = paths[P][M];

/**
 * 响应体类型：契约里所有字段都是可选的（后端响应 struct 不写 binding，swag 没有 required 可读），
 * 而响应**实际总是带这些字段**，所以递归去掉 `?` 和 `| undefined`。
 *
 * 为什么不直接用 `Required`：它只作用于顶层，嵌套对象里的字段仍然可选，
 * 业务侧照样要到处判空。
 */
export type RequiredSchema<K extends keyof components["schemas"]> =
  DeepRequired<components["schemas"][K]>;

/**
 * 递归地去掉所有 `?` 和 `| undefined`。
 *
 * 数组要单独处理（`DeepRequired<T[]> = DeepRequired<T>[]`），否则 `keyof T[]`
 * 会展开成数组的所有方法名。
 */
export type DeepRequired<T> = T extends (infer U)[]
  ? DeepRequired<U>[]
  : T extends object
    ? { [K in keyof T]-?: DeepRequired<Exclude<T[K], undefined>> }
    : Exclude<T, undefined>;

/**
 * 把指定字段放宽为可空。
 *
 * Go 里的 `*T` 字段会序列化成 `null`，但 swag **不表达空值**
 * （`nullable` / `type: [x, null]` 全仓 0 处）。所以"哪些字段真的是 null"
 * 只能在前端显式声明——好处是字段名写错、契约里删掉该字段都会编译报错。
 */
export type Nullable<T, K extends keyof T> = Omit<T, K> & { [P in K]: T[P] | null };

/** 把指定字段放回可选（后端确实可能不返回的字段）。 */
export type OptionalFields<T, K extends keyof T> = Omit<T, K> & { [P in K]?: T[P] };
