/**
 * 路由 query 的取值工具。
 *
 * 同一个 query 键可能因为重复参数变成数组（`?tab=chat&tab=site`），也可能被
 * Nuxt 归一化成数字（`?group=2`）。聊天草稿跳转与消息中心重定向各自复制过一份
 * 一模一样的第一项取值，这里合并成唯一实现：只取第一个可展示的标量。
 */
export function firstQueryValue(value: unknown): string {
  const raw = Array.isArray(value) ? value[0] : value;
  if (typeof raw === "string") return raw;
  if (typeof raw === "number") return String(raw);
  return "";
}
