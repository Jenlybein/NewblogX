/**
 * 契约里 kind 数值的**唯一**判定（#224）。
 *
 * 节点 kind：1 = 文件夹、2 = 文章节点；归属 kind：1 = 合集、2 = 项目。
 * 未知 / 缺失值**不猜默认类型**：判定函数一律返回 null，由调用方显式拒绝
 * （节点整行不渲染）或返回空（不生成来源链接）。
 */

/** 契约里节点 kind 的落库口径：1 = 文件夹。 */
const NODE_KIND_FOLDER = 1;
/** 契约里节点 kind 的落库口径：2 = 文章节点。 */
const NODE_KIND_ARTICLE = 2;

/** 契约里归属 kind 的落库口径：1 = 合集。 */
const OWNER_KIND_COLLECTION = 1;
/** 契约里归属 kind 的落库口径：2 = 项目。 */
const OWNER_KIND_PROJECT = 2;

/** 节点类型字面值（页面只区分文件夹与文章）。 */
export type OutlineNodeKind = "folder" | "article";

/** 来源类型字面值：与阅读页 `?from=<kind>:<id>` 的契约一致。 */
export type OutlineSourceKind = "project" | "collection";

/**
 * 节点 kind 数值 → 字面值；未知 / 缺失返回 null。
 *
 * 调用方必须显式处理 null（例如整行不渲染），不能默认成文章节点。
 */
export function outlineNodeKind(kind: number | null | undefined): OutlineNodeKind | null {
  switch (kind) {
    case NODE_KIND_FOLDER:
      return "folder";
    case NODE_KIND_ARTICLE:
      return "article";
    default:
      return null;
  }
}

/**
 * 归属 kind 数值 → 来源 kind；未知 / 缺失返回 null。
 *
 * 调用方必须显式处理 null（例如不生成 `?from=` 链接），不能猜一个默认归属。
 */
export function outlineSourceKind(ownerKind: number | null | undefined): OutlineSourceKind | null {
  switch (ownerKind) {
    case OWNER_KIND_COLLECTION:
      return "collection";
    case OWNER_KIND_PROJECT:
      return "project";
    default:
      return null;
  }
}

/** 来源字面值是否合法（解析 `?from=` 时用）。 */
export function isOutlineSourceKind(value: unknown): value is OutlineSourceKind {
  return value === "project" || value === "collection";
}
