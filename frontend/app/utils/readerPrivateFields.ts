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

/**
 * 文章页的**用户私有态**（ADR-0024 决策 2）。
 *
 * 只有这三项需要登录后单独取：`is_digg` / `is_favor` 决定按钮显示"已赞 / 已收藏"，
 * `relation` 是"我与作者的关系"（文章详情接口当前不返回它，作者关系由客户端的
 * `authorProfile` 源补齐；这里保留字段是为了与私有字段清单保持同一张表，
 * 缺省为 null 而不是伪造一个陌生人关系）。
 */
export interface ArticlePrivateState {
  is_digg: boolean;
  is_favor: boolean;
  relation: number | null;
}

/**
 * 从带鉴权的详情响应里**只投影**私有字段。
 *
 * 为什么不直接把整份详情塞给页面：详情里还有正文、计数、作者资料 —— 它们已经由
 * 匿名 payload / 30s SWR / 第二波作者源各自负责。私有态源只回答"我赞没赞、收没收藏"，
 * 它的值不进 payload、不进 SWR，也不与正文缓存混在一起。
 */
export function pickArticlePrivateState(value: object): ArticlePrivateState {
  const record = value as Record<string, unknown>;
  return {
    is_digg: Boolean(record.is_digg),
    is_favor: Boolean(record.is_favor),
    relation: typeof record.relation === "number" ? record.relation : null,
  };
}
