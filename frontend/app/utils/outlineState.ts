/**
 * 目录树展开/折叠状态（纯函数）。
 *
 * localStorage 里存的是「被收起的文件夹节点 ID」：默认全部展开，
 * 读者收起一个文件夹后，刷新或返回仍保持收起。集合按节点 ID 而不是标题，
 * 因为管理员可以重命名文件夹。
 */
import type { OutlineTreeRow } from "./outlineTree";

/** 渲染用的一行：在纯视图行上补一个「当前文章」标记。 */
export interface OutlineRowView extends OutlineTreeRow {
  isCurrent: boolean;
}

/** 给行打当前文章标记；文件夹永远不高亮。 */
export function markCurrentOutlineRows(
  rows: readonly OutlineTreeRow[],
  currentArticleId: string | null | undefined,
): OutlineRowView[] {
  return rows.map((row) => ({
    ...row,
    isCurrent: Boolean(currentArticleId) && row.articleId === currentArticleId,
  }));
}

/** 解析 localStorage 的旧值：坏数据一律当空集合，不让一个坏值毒死整棵树。 */
export function parseOutlineExpanded(raw: string | null | undefined): Set<string> {
  if (!raw) return new Set();
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return new Set();
    return new Set(
      parsed.filter((id): id is string => typeof id === "string" && id.length > 0),
    );
  } catch {
    return new Set();
  }
}

/** 序列化回 localStorage（稳定顺序便于比对与排查）。 */
export function serializeOutlineExpanded(collapsed: ReadonlySet<string>): string {
  return JSON.stringify([...collapsed].sort());
}

/**
 * 可见行：收起的文件夹自己仍显示（可再点开），它的后代整支隐藏。
 *
 * 当前文章没有特权（2026-11-10 按反馈修改）：读者收起哪一层，就保持哪一层；
 * 当前文章行跟着隐藏，不为高亮强行掰开文件夹。高亮只对"已经可见"的行生效。
 */
export function visibleOutlineRows(
  rows: readonly OutlineTreeRow[],
  collapsed: ReadonlySet<string> = new Set(),
): OutlineTreeRow[] {
  const visible: OutlineTreeRow[] = [];
  let hiddenBelow = -1;
  for (const row of rows) {
    if (hiddenBelow >= 0) {
      if (row.depth > hiddenBelow) continue;
      hiddenBelow = -1;
    }
    visible.push(row);
    if (row.isFolder && collapsed.has(row.id)) {
      hiddenBelow = row.depth;
    }
  }
  return visible;
}
