/**
 * 写客户端缓存前要剔除的用户私有字段（ADR-0019 约束 2 / 票 #85 验收项）。
 *
 * 为什么单独成文而不是在接入层就地删字段：私有字段清单会随契约变化，
 * 集中成一份清单 + 一个纯函数，既好测，也避免了"这里删了那里没删"。
 *
 * 只做**顶层浅拷贝**：文章正文 / 评论列表都是扁平对象，深拷贝会白付一份
 * 正文长度的拷贝成本，而缓存里本来也不该出现嵌套的私有字段。
 */
const PRIVATE_FIELDS = ["is_digg", "is_favor", "relation"] as const;

/** 需要剔除的私有字段清单（锁定它是为了让契约变化时有人被迫改这里）。 */
export const READER_PRIVATE_FIELDS = [...PRIVATE_FIELDS];

const PRIVATE_FIELD_SET = new Set<string>(PRIVATE_FIELDS);

/** 返回剔除私有字段后的浅拷贝；原对象保持不变（本次渲染仍需要完整数据）。 */
export function withoutPrivateFields<TValue extends object>(value: TValue): TValue {
  // 用 Object.fromEntries + 过滤而不是 delete：动态键 delete 会被 lint 拦，
  // 而且这里的数据都来自 JSON，滤掉一遍足够。
  const copy = Object.fromEntries(
    Object.entries(value).filter(([field]) => !PRIVATE_FIELD_SET.has(field)),
  );
  return copy as TValue;
}

/** 列表版：逐条剔除私有字段（评论 / 回复这类带 `is_digg` 的列表）。 */
export function withoutPrivateFieldsFromItems<TItem extends object>(items: readonly TItem[]): TItem[] {
  return items.map((item) => withoutPrivateFields(item));
}
