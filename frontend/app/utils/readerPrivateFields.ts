/**
 * 写客户端缓存前要剔除的用户私有字段（ADR-0016 约束 2 / 票 #85 验收项）。
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
 * 文章页的**用户私有态**（ADR-0016 决策 2；2026-11-04 后续修订扩到 4 字段）。
 *
 * `is_digg` / `is_favor` 决定按钮显示"已赞 / 已收藏"；`digg_count` / `favor_count`
 * 是**同一次带鉴权响应里的权威计数**。
 *
 * 为什么计数也走这里：硬刷新命中的是 30 秒匿名 SSR HTML，payload 里烤着写之前的数字，
 * hydration 又把它灌进客户端 30 秒 SWR；而私有态请求本来就打了一次带鉴权的详情、
 * 响应里就带着权威计数 —— 以前只投影两个布尔把它丢掉，于是"按钮已赞、数字没变"。
 * 四个字段一起回填，刷新路径上的数字才与按钮同源。
 *
 * 曾经的 `relation`（我与作者的关系）是**死字段**：文章详情接口不返回它，作者关系
 * 由客户端的 `authorProfile` 源补齐，写进这里只会永远是 null、诱导人去读。
 * 私有字段清单（`READER_PRIVATE_FIELDS`）里保留 `relation` 是因为评论条目确实带它，
 * 两份口径不要混为一谈。
 */
export interface ArticlePrivateState {
  is_digg: boolean;
  is_favor: boolean;
  /** 权威点赞数（与 is_digg 来自同一次响应）。 */
  digg_count: number;
  /** 权威收藏数（与 is_favor 来自同一次响应）。 */
  favor_count: number;
}

/** 计数兜底：非数字 / 负数 / NaN 一律归零，不给界面留一个 NaN。 */
function toCount(input: unknown): number {
  const value = Number(input);
  if (!Number.isFinite(value) || value <= 0) {
    return 0;
  }
  return Math.floor(value);
}

/**
 * 从带鉴权的详情响应里投影私有态（四个字段）。
 *
 * 只取这四个：详情里还有正文、作者资料 —— 它们已经由匿名 payload / 30s SWR /
 * 作者源各自负责。私有态的值不进 payload、不进 SWR，也不与正文缓存混在一起。
 */
export function pickArticlePrivateState(value: object): ArticlePrivateState {
  const record = value as Record<string, unknown>;
  return {
    is_digg: Boolean(record.is_digg),
    is_favor: Boolean(record.is_favor),
    digg_count: toCount(record.digg_count),
    favor_count: toCount(record.favor_count),
  };
}
