/**
 * OpenAPI 契约类型入口（S2.9）。
 *
 * 链路：backend/docs/swagger.json（swag 从代码注解生成）
 *   → `pnpm openapi:types`（swagger2openapi 转 3.0 + openapi-typescript）
 *   → ./openapi.d.ts（自动生成，勿手改）
 *
 * 约定：
 * - 新写 API 调用一律从这里取请求/响应类型：`Schema<"...">` 或 `Operation<"...", "get">`。
 * - `api.ts` 保留为「视图模型 / 适配层」：组件直接消费面向 UI 的领域形状，
 *   不再用它镜像原始 HTTP 契约。
 */
import type { components, paths } from "./openapi";

/** 取某个 schema 的类型，如 `Schema<"api_banner_api.BannerCreateRequest">`。 */
export type Schema<K extends keyof components["schemas"]> = components["schemas"][K];

/** 取某个操作的类型，如 `Operation<"/banners", "post">["responses"]`。 */
export type Operation<P extends keyof paths, M extends keyof paths[P]> = paths[P][M];
