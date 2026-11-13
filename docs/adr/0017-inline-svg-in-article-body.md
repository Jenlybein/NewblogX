# ADR-0017：文章正文放行内联 SVG（两层白名单）

- 状态：已接受
- 日期：2026-11-02
- 来源：用户反馈「markdown 无法显示 svg 图」· 2026-11-02 设计评审结论（纯文本记录）

## 背景与问题

文章正文里存在「`<div class="svg-block">` 包 `<svg>`，配 `<center>` 图注」这类图示写法。要让内联 SVG 既能存进库又能显示成图，必须同时跨过两道关卡，任改一处都不生效：

1. **后端入库消毒**。`MdToSafe`（`backend/utils/markdown/`）以 `bluemonday.UGCPolicy()` 为基础策略，而 UGC 策略本身不含 svg：整块 `<svg>` 会被删除（不是转义），`class` 被剥掉，`<center>` 被剥空，图根本进不了数据库。
2. **前端渲染解析与消毒**。markdown-it 若关闭 HTML，库里即便有 SVG 也只会输出转义后的源码文本；而放开内联 HTML 又必须再用一层白名单清洗，否则文章正文这个"任何人可能写、所有读者都会执行"的位置会成为存储型 XSS 的入口。

另有两点边界：`![x](https://…/a.svg)` 图片语法本来就走 `<img>`、由浏览器原生显示；上传 SVG 则受格式白名单约束（`site_default_settings.yaml` 默认白名单无 svg）。这两条是不同需求路径，不在本 ADR 范围内。

## 决策

1. **两层都放行，且都是"加白名单"而不是"关掉消毒"**：
   - 后端：[md_safe_depends.go](../../backend/utils/markdown/md_safe_depends.go) 在 bluemonday 策略里放行 SVG 绘图元素与属性，让 SVG 存得进库；无属性的绘图元素也要显式放行（否则容器标签会被剥成纯文本，图散架只剩文字）。
   - 前端：[useArticleMarkdown.ts](../../frontend/app/composables/useArticleMarkdown.ts) 开 `html: true`，渲染结果必须经 [sanitizeHtml.ts](../../frontend/app/utils/sanitizeHtml.ts) 的 `sanitizeArticleHtml`（DOMPurify 白名单，关闭 svgFilters）清洗后再注入。
   两层必须同时放开同一份口径，否则会出现"存进去了但不显示"或"显示了但存不住"。

2. **放行范围 = 绘图元素 + 少量排版标签**。允许 `svg` 及其绘图子元素（`g/title/desc/rect/circle/ellipse/line/polyline/polygon/path/text/tspan`）与绘图属性（`viewBox/d/fill/stroke/stroke-width/…` 以及 `xmlns`、`id`、`class`、`style` 等）；同时放行 `div/center/figure/figcaption` 排版标签并保留 `class`，以支持"图 + 图注"的现有写法。理由：用户内容里就带 `<div class>` 与 `<center>`，只放行 `svg` 会迫使改写作习惯，且图注会退化成普通段落；放行这几个标签的增量攻击面可控。

3. **消毒粒度 = 严格白名单，只放行绘图元素**。明确禁止：`script`、`foreignObject`、`animate` / `animateTransform` 等特效与滤镜元素、所有 `on*` 事件属性、`href` / `xlink:href` 中的非绘图协议。理由：SVG 是可执行脚本的向量——它可内嵌 `<script>`、`onload`、`javascript:` 链接，是存储型 XSS 的常见载体。用户实际的流程图、架构图完全由绘图元素构成，严格白名单不影响表达力。

4. **存量不迁移**。已发布文章里的 SVG 在当初保存时就没进数据库；若保有原始 markdown，重新保存一次即可。本决策只约束此后写入与渲染的文章正文。

## 理由与已否决方案

- **`html: true` 但不消毒**：等于开放任意 HTML，任何能发文的人都能塞 `<script>`，读者打开文章即中招。已否决。
- **只支持 ```svg 围栏代码块**（对标现有 mermaid 特判）：不开放任意 HTML 最安全，但要求改写所有存量内容，且 `<div>` / `<center>` 的排版能力丢失。为保留现有写作方式而否决。
- **只在单层放行**：仅在后端放行则前端仍当文本，仅在前端放行则库里根本没有。已否决。
- **允许上传 SVG 文件**：与"正文内联 SVG"是不同的需求路径（上传涉及格式白名单与存储侧审核）；本 ADR 不做。

## 代价 / 不做什么

- 新增前端依赖（DOMPurify），且每次渲染多一道清洗；读者站与后台编辑器预览**共用同一条渲染管线**，两处一起生效，避免"读者站能显示、后台预览显示源码"的割裂。
- 后端放行 SVG 后，`MdToSafe` 的输出不再是"纯 markdown + 极少量 HTML"，下游（ES 投影、摘要生成、引用账本的 `<img src>` 解析）需要确认不受影响。
- 白名单是**逐步收紧容易、放宽难**：日后要加特效元素须重走本 ADR。
- **不做什么**：不迁移存量文章、不放行 SVG 上传、不引入围栏代码块特判、不把消毒改成允许任意 HTML、不把两层白名单合并成一层的"全量放行"。

## 当前生效约束

1. 后端入库与前端渲染**两层必须同时**按同一份口径放行内联 SVG；不得只改一层，也不得在任何一层关掉消毒。
2. `MdToSafe` 的 bluemonday 策略只允许白名单内的 SVG 绘图元素与属性、以及 `div/center/figure/figcaption` 排版标签（保留 `class`）；`script`、`foreignObject`、`animate*`、`on*`、`javascript:` 等非绘图协议一律不得放行。
3. `useArticleMarkdown.ts` 必须保持 `html: true` 并在注入前调用 `sanitizeArticleHtml`；读者站与后台编辑器预览共用这条渲染管线，不得各自维护一份白名单。
4. 存量文章不做数据迁移；需要显示 SVG 时由作者用原始 markdown 重新保存。
5. 白名单只可收紧；新增特效元素、滤镜或协议必须另开 ADR 并重验 XSS 面。
6. 图片语法引用的 `.svg` 与上传 SVG 不在本 ADR 范围内；上传格式白名单仍不含 svg。

## 相关代码与文档

- 后端消毒：[md_safe_depends.go](../../backend/utils/markdown/md_safe_depends.go)（`svgDrawingElements` / `svgDrawingAttrs` / `layoutElements`）· [enter.go](../../backend/utils/markdown/enter.go)（`MdToSafe`）· [markdown_test.go](../../backend/utils/markdown/markdown_test.go)（放行与拦截用例）
- 前端渲染与清洗：[useArticleMarkdown.ts](../../frontend/app/composables/useArticleMarkdown.ts) · [sanitizeHtml.ts](../../frontend/app/utils/sanitizeHtml.ts) · [useArticleMarkdown.test.ts](../../frontend/test/composables/useArticleMarkdown.test.ts) · [sanitizeHtml.test.ts](../../frontend/test/utils/sanitizeHtml.test.ts) · [MarkdownRenderSurface.test.ts](../../frontend/test/components/MarkdownRenderSurface.test.ts)
- 上传白名单：[site_default_settings.yaml](../../backend/config/site_default_settings.yaml)
- 词汇：[GLOSSARY](../../GLOSSARY.md)
