/**
 * 列表类端点响应的读取入口（`kernel.ListData[T]` → 条目数组 + 总数）。
 *
 * 实现已搬到 `~/utils/listPayload`：那是纯函数，`utils/taxonomyOptions` 的映射也要
 * 按同一口径读形状，而依赖方向只能是 `services → utils`（utils 反向 import
 * services 会成环）。这里保留为 services 层的入口，既有调用点（含用例的 import）
 * 不必改动，口径也只有一份。
 */
export { readListPayload } from "~/utils/listPayload";
export type { ListPayload, ListPayloadItems } from "~/utils/listPayload";
