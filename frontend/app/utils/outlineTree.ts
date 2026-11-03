/**
 * 目录树（读者侧）的纯映射。
 *
 * 后端 `OutlineTreeResponse` 是一棵嵌套树（文件夹 + 文章节点），页面要的是
 * 「一行 + 层级 + 折叠箭头 + 带来源的文章链接」。这里把遍历、层级与来源参数
 * 收成一份可测的纯函数：合集与项目两个详情页共用，折叠只影响可见行、不改写
 * 这份结果。
 *
 * 「第 N 篇」位置标记已随扁平列表退役：树里位置靠缩进表达（notes A7）。
 */

/** 目录树节点（契约字段的子集：页面只消费这些）。 */
export interface OutlineNode {
  id?: string;
  kind: number;
  title?: string;
  article_id?: string;
  article_title?: string;
  article_cover?: string;
  children?: OutlineNode[] | null;
}

/** 来源类型字面值：与阅读页 `?from=<kind>:<id>` 的契约一致。 */
export type OutlineSourceKind = "project" | "collection";

export interface OutlineSource {
  kind: OutlineSourceKind;
  id: string;
}

/** 详情页目录树的一行。 */
export interface OutlineTreeRow {
  /** 节点 ID：折叠状态的稳定键（文件夹标题可改，ID 不会）。 */
  id: string;
  /** 文件夹节点：可折叠、不可点击、不带文章链接。 */
  isFolder: boolean;
  /** 有子节点：文件夹要画展开/收起箭头。 */
  hasChildren: boolean;
  /** 标题：文件夹用自己的标题，文章节点读文章快照的标题。 */
  label: string;
  /** 封面：只文章节点有；空串交给 ArticleCover 的透明空白。 */
  cover: string;
  /** 层级（根 = 0）：缩进与引导线由页面按它渲染。 */
  depth: number;
  /** 缩进档位（与 depth 同源，显式给样式用）。 */
  indent: number;
  /** 文章节点：带 `?from=<kind>:<id>` 的阅读页链接。 */
  href?: string;
  /** 文章节点所指文章 ID（高亮当前文章用）。 */
  articleId?: string;
}

/** 生成来源参数：`<kind>:<id>`（如 `project:375`）。 */
export function outlineSource(kind: OutlineSourceKind, id: string | number): string {
  return `${kind}:${encodeURIComponent(String(id))}`;
}

/** 解析来源参数；不完整、kind 不认识或 ID 为空都视为没有来源。 */
export function parseOutlineSource(raw: string | null | undefined): OutlineSource | null {
  if (!raw) return null;
  const match = /^(project|collection):([^:]+)$/u.exec(raw);
  if (!match) return null;
  const id = match[2]!.trim();
  if (!id) return null;
  return { kind: match[1] as OutlineSourceKind, id };
}

/**
 * 文章节点链接：路径只放文章 ID，来源放查询串。
 *
 * `from` 由 `outlineSource` 生成，冒号在查询串里是合法字符，保持字面
 * `project:375` 的形态便于分享与阅读页解析（不编码成 `%3A`）。
 */
export function outlineArticleHref(articleId: string | number, from: string): string {
  return `/article/${encodeURIComponent(String(articleId))}?from=${from}`;
}

/**
 * 嵌套树 → 深度优先的行。
 *
 * 只公开文章节点会出现在读者侧（后端已按公开口径过滤），空文件夹保留：
 * 它表达「先搭章节骨架」，读者看到标题但没有文章可点。
 */
export function outlineTreeRows(
  nodes: OutlineNode[] | null | undefined,
  kind: OutlineSourceKind,
  ownerId: string | number,
): OutlineTreeRow[] {
  const from = outlineSource(kind, ownerId);
  const rows: OutlineTreeRow[] = [];

  const walk = (items: OutlineNode[] | null | undefined, depth: number): void => {
    for (const node of items ?? []) {
      const children = node.children ?? [];
      const isFolder = node.kind === 1;
      const label = isFolder
        ? (node.title ?? "").trim() || "未命名章节"
        : (node.article_title ?? "").trim() || "未命名文章";
      const articleId = isFolder ? "" : String(node.article_id ?? "").trim();

      rows.push({
        id: String(node.id ?? articleId ?? ""),
        isFolder,
        hasChildren: children.length > 0,
        label,
        cover: isFolder ? "" : (node.article_cover ?? "").trim(),
        depth,
        indent: depth,
        ...(isFolder || !articleId
          ? {}
          : {
              href: outlineArticleHref(articleId, from),
              articleId,
            }),
      });
      walk(children, depth + 1);
    }
  };

  walk(nodes, 0);
  return rows;
}

/** 篇数只数文章节点（文件夹不计，与后端「只统计公开文章」同一口径）。 */
export function outlineTreeCount(nodes: OutlineNode[] | null | undefined): number {
  let count = 0;
  const walk = (items: OutlineNode[] | null | undefined): void => {
    for (const node of items ?? []) {
      if (node.kind !== 1) count += 1;
      walk(node.children);
    }
  };
  walk(nodes);
  return count;
}

/**
 * 封面回退：深度优先第一篇**有封面的公开文章节点**的封面。
 *
 * 树里全是公开文章（失效节点后端已过滤）；没有封面或没有文章时给空串，
 * 让共用封面组件渲染透明空白，不编造占位图。
 */
export function firstPublicArticleCover(nodes: OutlineNode[] | null | undefined): string {
  for (const node of nodes ?? []) {
    if (node.kind !== 1) {
      const cover = (node.article_cover ?? "").trim();
      if (cover) return cover;
    }
    const nested = firstPublicArticleCover(node.children);
    if (nested) return nested;
  }
  return "";
}
