import type { OptionItem } from "~/types/api";

// 分类 / 标签下拉项的映射（#320）。
//
// 抽出来的理由：两个 options 端点的 data 恒为 `{list, count}`（kernel.ListData），
// 而 `request()` 返回的是**已解包的 data**。把 data 当数组 `.map` 会抛
// `list.map is not a function`，再被调用点的 `.catch(() => [])` 吞掉 ——
// 用户看到的就是永远空白、一声不响的下拉，于是重复建同名分类。
//
// 映射只在这里实现一次：写作页的分类下拉、标签下拉，以及后续的分类栏都走它。

/** 选项源条目：契约里 `id` / `title` 都是可选（swag 不写 required），缺一半就用不了。 */
export type TaxonomyOptionSource = {
  id?: string;
  title?: string;
};

/** 选项载荷：`kernel.ListData` 的 `{list, count}`；连 list 都可能缺。 */
export type TaxonomyOptionsPayload =
  | {
      list?: Array<TaxonomyOptionSource | null> | null;
    }
  | null
  | undefined;

/**
 * toOptionItems 把列表载荷映射成下拉项：标题做 label，ID 做 value。
 *
 * 三条规矩：
 *   1. 缺 ID 或标题的条目直接跳过 —— 没有 ID 选不中，没有标题看不出是什么；
 *   2. 载荷缺 list（后端返回空 data）时给空数组，不抛错；
 *   3. 只做映射，不判断"该不该显示空状态 / 错误"——那是调用点的事，
 *      因为"一个分类都没有"和"接口挂了"要给用户看不同的话。
 */
export function toOptionItems(payload: TaxonomyOptionsPayload): OptionItem[] {
  const items = payload?.list;
  if (!Array.isArray(items)) return [];

  const options: OptionItem[] = [];
  for (const item of items) {
    const value = item?.id?.trim() ?? "";
    const label = item?.title?.trim() ?? "";
    if (!value || !label) continue;
    options.push({ label, value });
  }
  return options;
}

/** 待匹配的目标：AI 元信息补全返回的是 `{id?, title?}`（两者都可能缺）。 */
export type TaxonomyOptionMatch = {
  id?: string;
  title?: string;
};

/**
 * matchOptionByText 把"名字或 ID"对上既有下拉项，返回它的 value（选项 ID）。
 *
 * 顺序是刻意的：先认 ID（准确），ID 对不上再认标题（忽略大小写与首尾空白，
 * 因为 AI 返回的是人写的名字）。都对不上就返回 null —— 调用方必须当作
 * "没命中"，**绝不**把 AI 给的名字当 ID 写进表单。
 */
export function matchOptionByText(
  options: ReadonlyArray<OptionItem>,
  target?: TaxonomyOptionMatch | null,
): string | null {
  if (!target) return null;

  if (target.id) {
    const byId = options.find((option) => option.value === target.id);
    if (byId) return byId.value;
  }

  const title = target.title?.trim().toLowerCase();
  if (!title) return null;

  const byTitle = options.find((option) => option.label.trim().toLowerCase() === title);
  return byTitle?.value ?? null;
}
