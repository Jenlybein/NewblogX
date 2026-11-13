import DOMPurify from "isomorphic-dompurify";

/**
 * 全站 HTML 清洗的**唯一**白名单入口。
 *
 * # 为什么有两个档位而不是两套白名单
 *
 * 读者站有两处把 HTML 直接交给浏览器：文章正文（markdown 渲染后的结果，可能夹带
 * 原生 HTML 与内联 SVG）和右栏富文本卡（运营在后台直接写 HTML）。两处的信任级别
 * 不同，但**允许标签的维护责任是同一份**：如果各自抄一份 `FORBID_TAGS`，迟早一边
 * 补了漏洞另一边没补。所以这里保留 DOMPurify 自带的、由上游维护的 HTML/SVG 允许
 * 清单，只按档位**收窄**，不另起一份手写白名单。
 *
 * - `article`：正文档。放行内联 SVG（ADR-0017），只禁掉 SVG 里能装 HTML、能加载
 *   外部资源、能跑动画的那批标签。
 * - `rich_text`：右栏富文本档。只走 HTML 档、不放 SVG；脚本 / 内联事件 / iframe /
 *   表单 / 媒体 / `data:` 伪协议一律剥离。运营只需要基础排版（标题、粗斜体、列表、
 *   链接、引用、代码），不需要任何能执行、能外联或能盖住版面的东西。
 *
 * SSR 与浏览器都要跑，所以用 isomorphic 版本（Node 侧自带 DOM 实现）。
 */
export type HtmlSanitizeProfile = "article" | "rich_text";

// SVG 里刻意禁止的面。
//
// 开 html:true 之后正文里的原始 HTML 会原样进入渲染结果，所以必须消毒。
// DOMPurify 默认已拦掉所有 on* 事件属性与 javascript: 协议，但它的 SVG
// 允许清单**包含**特效与装载面，这些要显式禁掉：
//   - foreignObject：能装任意 HTML（等于绕过整个 HTML 白名单）
//   - animate* / filter / mask / clipPath：动画与滤镜，是混淆攻击的常见载体
//   - image / symbol / switch / textPath / metadata：能让 SVG 引用外部资源
const ARTICLE_FORBIDDEN_TAGS = [
  "foreignObject",
  "animate",
  "animateMotion",
  "animateTransform",
  "filter",
  "mask",
  "clipPath",
  "image",
  "symbol",
  "switch",
  "textPath",
  "metadata",
];

/**
 * 富文本档额外禁掉的标签：都是"能执行 / 能外联 / 能盖住版面 / 能提交"的面。
 *
 * 图片与外链缩略图一并禁掉：右栏只有 310px 宽，运营需要图时应该走封面类字段；
 * 允许 `<img src="data:...">` 等于把 `data:` 伪协议从后门放回来。DOMPurify 对
 * `img` 的 `src` 有专门的数据 URI 直通分支，禁标签比逐个抠属性更稳。
 */
const RICH_TEXT_FORBIDDEN_TAGS = [
  "script",
  "style",
  "iframe",
  "frame",
  "frameset",
  "object",
  "embed",
  "applet",
  "form",
  "input",
  "button",
  "select",
  "option",
  "optgroup",
  "textarea",
  "audio",
  "video",
  "source",
  "track",
  "picture",
  "img",
  "canvas",
  "svg",
  "math",
  "template",
  "meta",
  "link",
  "base",
  "html",
  "head",
  "title",
  "body",
  "dialog",
  "marquee",
  "blink",
  "xmp",
  "plaintext",
  "noscript",
  "noembed",
  "noframes",
];

/**
 * 富文本档允许的链接协议：只放 http(s)、mailto、tel，以及相对地址与页内锚点。
 *
 * 正则形制与 DOMPurify 默认的 `ALLOWED_URI_REGEXP` 一致（第三分支专门放行
 * 相对地址：首字符非字母，或整串不含冒号）。区别是这里不再放行
 * `callto` / `sms` / `cid` / `xmpp` 这些用不上的协议。
 */
const RICH_TEXT_ALLOWED_URI = /^(?:(?:(?:f|ht)tps?|mailto|tel):|[^a-z]|[a-z+.-]+(?:[^a-z+.:-]|$))/iu;

/** 正文档清洗：行为与既有的 `sanitizeArticleHtml` 完全一致。 */
function sanitizeArticleHtmlImpl(html: string): string {
  return DOMPurify.sanitize(html, {
    USE_PROFILES: { html: true, svg: true, svgFilters: false },
    FORBID_TAGS: ARTICLE_FORBIDDEN_TAGS,
  });
}

/** 富文本档清洗：只走 HTML 档，并收窄到基础排版。 */
function sanitizeRichTextHtmlImpl(html: string): string {
  const sanitized = DOMPurify.sanitize(html, {
    USE_PROFILES: { html: true },
    FORBID_TAGS: RICH_TEXT_FORBIDDEN_TAGS,
    FORBID_ATTR: ["style"],
    ALLOWED_URI_REGEXP: RICH_TEXT_ALLOWED_URI,
  });
  // 清洗后只剩空白（或只剩被剥掉的标签）等于没有内容：右栏据此整张卡隐藏。
  return hasVisibleRichTextHtml(sanitized) ? sanitized : "";
}

/**
 * 判断清洗后的富文本是否还有肉眼可见内容。
 *
 * 只看文本节点：富文本档禁掉了 img / svg / video 这类"没有文本也能占地"的标签，
 * 所以去标签后为空就真的是一片空白。`&nbsp;` 不算内容。
 */
function hasVisibleRichTextHtml(html: string): boolean {
  return html.replace(/<[^>]+>/gu, "").replace(/&nbsp;/giu, " ").trim().length > 0;
}

/** 按档位清洗一段 HTML。默认走正文档，避免调用方漏传档位时落到更宽的配置上。 */
export function sanitizeHtml(html: string, profile: HtmlSanitizeProfile = "article"): string {
  if (!html) {
    return "";
  }
  return profile === "rich_text" ? sanitizeRichTextHtmlImpl(html) : sanitizeArticleHtmlImpl(html);
}

/** 正文档清洗（文章正文渲染用；与历史行为逐字一致）。 */
export function sanitizeArticleHtml(html: string): string {
  return sanitizeHtml(html, "article");
}

/** 富文本档清洗（右栏富文本卡用）。 */
export function sanitizeRichTextHtml(html: string): string {
  return sanitizeHtml(html, "rich_text");
}
