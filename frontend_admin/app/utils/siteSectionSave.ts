/**
 * site 段的「读-改-写」保存语义（ADR-0029）。
 *
 * 站点段拆成四页之后，每页仍然读写**同一个**整段端点：读 `GET /site/admin/site`、
 * 写 `PUT /site/site`（整体覆盖）。如果每页把自己加载时的那份整段原样写回，
 * 两个页面同时开着时后保存的那页就会把前一页的改动覆盖掉；更糟的是
 * `about` / `project` / `reconcile` 这三个后台没有 UI 的块，一旦某页只提交
 * 自己那几块，Go 的绑定会把缺席的块当零值，等于把它们清空。
 *
 * 所以保存固定走三步：**取最新整段 → 只覆盖本页拥有的顶层块 → 写回**。
 * 本文件只做中间那一步的纯计算，方便单测；取与写由页面自己调服务层。
 */

/**
 * 用 patched 里指定块的**部分字段**覆盖 latest 同名块的对应字段，其余字段原样保留。
 *
 * 为什么需要比"块级"更细一层：2026-11-27 把「文章发布」拆成两页
 * （发布与展示 / 文章审核设置），两页都写 site 段的同一个 `article` 块。
 * 若仍用块级替换，后保存的那页会把自己加载时的旧快照整体写回 ——
 * 兄弟页刚改的免审核开关或质量闸门会被悄悄覆盖掉。
 *
 * 规则与块级版一致：`fields` 里为 `undefined` 的字段不覆盖；不原地修改入参。
 */
export function applySiteBlockFieldPatch<T extends object>(
  latest: T,
  patched: T,
  block: keyof T,
  fields: readonly string[],
): T {
  const merged: T = { ...latest }
  const latestBlock = latest[block] as Record<string, unknown> | undefined
  const patchedBlock = patched[block] as Record<string, unknown> | undefined
  if (patchedBlock === undefined) return merged

  const next: Record<string, unknown> = { ...(latestBlock ?? {}) }
  for (const field of fields) {
    const value = patchedBlock[field]
    if (value === undefined) continue
    next[field] = value
  }
  ;(merged as Record<string, unknown>)[block as string] = next
  return merged
}

/**
 * 用 patched 里指定的顶层块覆盖 latest，其余块原样保留（返回新对象）。
 *
 * 两条额外规则：
 *   - `patched` 里为 `undefined` 的块**不覆盖**：旧库还没补种该块时，
 *     页面加载到的就是 undefined，此时保留 latest 的值比写成空值安全；
 *   - 不原地改 `latest` / `patched`：调用方可能还持有它们（例如表单正在编辑）。
 */
export function applySiteSectionPatch<T extends object>(
  latest: T,
  patched: T,
  blocks: readonly (keyof T)[],
): T {
  const merged: T = { ...latest }
  for (const block of blocks) {
    if (patched[block] === undefined) continue
    merged[block] = patched[block]
  }
  return merged
}
