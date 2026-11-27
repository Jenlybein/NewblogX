/**
 * 一篇文章最多可选几个标签的**兜底值**。
 *
 * 真值来自站点配置 `site.article.max_tags_per_article`（后台「文章发布 → 发布约束」可改，
 * 后端 `domain.NormalizeTagLimit` 把越界值钳回默认 5）。写稿页读站点配置，读不到
 * （老库合并默认值之前 / 配置请求失败）才回落到这个常量 —— 否则一次配置读取失败
 * 就会让作者选不了标签。
 *
 * 真正拦人的仍是后端（超过上限返回 `article.tag_limit_exceeded`）；这里只负责
 * 把选项提前置灰、把"最多几个"说清楚。
 */
export const MAX_ARTICLE_TAGS = 5;
