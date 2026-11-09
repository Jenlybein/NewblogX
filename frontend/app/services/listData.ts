/**
 * 列表类端点的响应读取（`kernel.ListData[T]` → 条目数组）。
 *
 * 为什么单独一个模块：列表端点的 `data` 恒为 `{list, count}`（后端
 * `kernel.ListData` + `res.OkWithList`），而 `$api.request()` 返回的是**已解包的 data**。
 * 所以拿到的不是数组，直接 `.map` 会抛 `list.map is not a function`（#320 的根因：
 * 声明成 `Array<...>` 又在错误处理里 `.catch(() => [])`，错误被吞掉、下拉永远空）。
 *
 * 这里把"从列表响应里取条目"收敛成唯一读法，各服务复用它，不再各写一份形状假设。
 * 只读 `list` 与 `count`，不认别的键：契约里列表包装就这两个字段。
 */

/** `kernel.ListData[T]` 的最小形状（字段都可能缺席：降级响应 / 空 data）。 */
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
 * 而不是在模板渲染中途炸掉。`count` 缺席时回落到条目数。
 */
export function readListPayload<T>(payload: ListPayload<T> | null | undefined): ListPayloadItems<T> {
  const list = Array.isArray(payload?.list) ? payload.list : [];
  const count = typeof payload?.count === "number" ? payload.count : list.length;

  return { list, count };
}
