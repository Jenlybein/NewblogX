import type { VendorItem } from '~/types/api'

/**
 * 用供应商的字段定义生成表单初值。
 *
 * 抽成纯函数而不是留在组件里：新增与编辑两条路径都要用它，而且编辑时
 * 还要把"已填的值"盖在默认值上面——这段逻辑写在组件里就只能靠挂载组件来测。
 */
export function vendorFieldDefaults(vendor: VendorItem | null | undefined): Record<string, string> {
  const out: Record<string, string> = {}
  for (const field of vendor?.fields ?? []) {
    out[field.key] = field.default || ''
  }
  return out
}

/**
 * 编辑时的初始化：已有值优先，缺的用默认值补齐。
 *
 * 密钥字段回显的是后端遮蔽后的 ******，原样带回去表示"没改"，
 * 后端按占位符识别并保留库里的真实密钥。
 */
export function mergeVendorFields(
  vendor: VendorItem | null | undefined,
  current: Record<string, string> | undefined,
): Record<string, string> {
  return { ...vendorFieldDefaults(vendor), ...(current ?? {}) }
}
