import type { OptionItem } from "~/types/api";

// 写作页"分类"下拉的纯规则（#127）。
//
// 抽出来的理由：内联新建分类的输入框在弹窗里，而"标题去空白、空标题拒绝、
// 创建后在下拉里找到并选中"这三条判断属于文章数据域（useStudioArticle）。
// 两处都只看规则、不碰网络与组件，所以放在纯函数里各用一次。

/** 分类下拉项（与后端选项接口映射后的形状一致）。 */
export type CategoryOption = OptionItem;

/** 统一分类标题：首尾空白不算标题的一部分。 */
export function normalizeCategoryTitle(raw: string): string {
  return raw.trim();
}

/** 标题为空时返回给用户看的原因；合法时返回 null。 */
export function categoryTitleError(raw: string): string | null {
  return normalizeCategoryTitle(raw) ? null : "请输入分类名称";
}

/** 在下拉项里按标题找到刚创建的分类；找不到返回 null。 */
export function findCategoryOptionByTitle(options: CategoryOption[], rawTitle: string): CategoryOption | null {
  const title = normalizeCategoryTitle(rawTitle);
  if (!title) return null;
  return options.find((option) => option.label === title) ?? null;
}
