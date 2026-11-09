/**
 * 列表类端点的响应读取（`kernel.ListData[T]` → 条目数组 + 总数）。
 *
 * 为什么放在 utils/（纯函数，无请求、无 Nuxt 上下文）：
 * `services/listData` 曾经是唯一实现，但 `utils/taxonomyOptions` 的映射也要按同一
 * 口径读形状，而依赖方向只能是 `services → utils`，反向 import 会形成环。搬到
 * utils 后两处共用一份，services 里只保留兼容再导出。
 *
 * 为什么必须收口：列表端点的 `data` 恒为 `{list, count}`（后端 `kernel.ListData` +
 * `res.OkWithList`），而 `$api.request()` 返回的是**已解包的 data**。拿到手的不是
 * 数组，直接 `.map` 会抛 `list.map is not a function`，再被调用点的
 * `.catch(() => [])` 吞掉 —— 用户看到的就是永远空白的下拉（#320 的病根）。
 * 此前三处各写了一遍 `Array.isArray(payload?.list)`，口径改一次要改三处。
 *
 * 只认 `list` 与 `count` 两个键，不猜别的名字：契约里列表包装就这两个字段。
 */

/** `kernel.ListData[T]` 的最小形状（字段都可能缺：降级响应 / 空 data）。 */
export interface ListPayload<T> {
  list?: T[] | null;
  count?: number | null;
}

export interface ListPayloadItems<T> {
  list: T[];
  count: number;
}

/**
 * 读列表响应：条目数组 + 总数。
 *
 * 非数组的 `list`（缺失、null、后端降级成 `{}`）统一读成空列表，让调用方进空态，
 * 而不是在模板渲染中途炸掉。`count` 缺失时回落到条目数。
 */
export function readListPayload<T>(payload: ListPayload<T> | null | undefined): ListPayloadItems<T> {
  const list = Array.isArray(payload?.list) ? payload.list : [];
  const count = typeof payload?.count === "number" ? payload.count : list.length;

  return { list, count };
}
