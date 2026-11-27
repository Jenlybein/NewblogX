/**
 * 一篇文章最多可选的标签数。
 *
 * 与后端 `article/domain.MaxArticleTags` 是同一个数字，两边各写一份：
 * 前端据它禁用多余选项、给作者一句明确的提示，后端才是唯一真相
 * （超过上限的请求一律返回 `article.tag_limit_exceeded`）。
 *
 * 为什么不从接口读：契约里 `tag_ids` 只是个字符串数组，没有 maxItems；
 * 就算后端加了，生成的类型也表达不出"这个数字"这层语义。写一份常量
 * 在两端各自维护，比让前端为了一句提示多一次启动期请求便宜。
 */
export const MAX_ARTICLE_TAGS = 5;

/** 给作者看的上限提示（发布弹窗里紧挨着"添加标签"）。 */
export const MAX_ARTICLE_TAGS_HINT = `最多选 ${MAX_ARTICLE_TAGS} 个标签`;
