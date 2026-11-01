# ADR-0021：文章正文放行内联 SVG（两层白名单）

- 状态：已定案待实施
- 日期：2026-11-02
- 来源：用户反馈「markdown 无法显示 svg 图」· grill 结论

## 背景与问题

用户在文章正文里写 SVG 图示（`<div class="svg-block">` 包 `<svg>`，配 `<center>` 图注），渲染出来是**源码文本**，不是图。

实测定位到**两处独立的拦截**，任改一处都不生效：

1. **后端保存时 SVG 就已经丢失**。`MdToSafe`（`backend/utils/markdown/`）用 `bluemonday.UGCPolicy()` 消毒，而 UGC 策略**不含 svg**。实测把原文喂进去：

   ```
   输入: <div class="svg-block"><svg viewBox=...><circle .../></svg><center>图 1</center></div>
   存储: <div>

          图 1
          </div>
   ```

   `<svg>` 被**整块删除**（不是转义），`class` 被剥掉，`<center>` 被剥空。**图压根没进数据库**。

2. **前端也不解析内联 HTML**。`useArticleMarkdown.ts` 的 markdown-it 配 `html: false`，即使库里有 SVG 也只会被转义成文本输出。

另有两点边界：`![x](https://…/a.svg)` 图片语法**本来就能显示**（生成 `<img>`，浏览器原生支持 svg）；上传 SVG 则被格式白名单拒绝（`site_default_settings.yaml` 默认白名单无 svg）——这两条不在本次范围内。

## 决策

1. **两层都改，且都是"加白名单"而不是"关掉消毒"**：
   - 后端：`md_safe_depends.go` 在 bluemonday 策略里放行 SVG 绘图元素与属性，让 SVG 存得进库
   - 前端：`useArticleMarkdown.ts` 开 `html: true`，渲染后经 DOMPurify 按白名单清洗再注入
   理由：只改后端 → 前端仍当文本；只改前端 → 库里根本没有。两层必须同时放开同一份口径，否则会出现"存进去了但不显示"或"显示了但存不住"。

2. **放行范围 = 绘图元素 + 少量排版标签**。允许 `svg` 及其绘图子元素（`g/title/desc/rect/circle/ellipse/line/polyline/polygon/path/text/tspan` 等）与绘图属性（`viewBox/d/fill/stroke/stroke-width/…`）；同时放行 `div/center/figure/figcaption` 等排版标签并保留 `class`，以支持"图 + 图注"的现有写法。
   理由：用户的内容里就带 `<div class>` 与 `<center>`，只放行 `svg` 会迫使改写作习惯，且图注会退化成普通段落。放行这几个标签的增量攻击面可控。

3. **消毒粒度 = 严格白名单，只放行绘图元素**。明确禁止：`script`、`foreignObject`、`animate`/`animateTransform` 等特效元素、所有 `on*` 事件属性、`href`/`xlink:href` 中的非绘图协议。
   理由：**SVG 是可执行脚本的向量**——它可内嵌 `<script>`、`onload`、`javascript:` 链接，是存储型 XSS 的常见载体。文章正文是"任何人可能写、所有读者都会执行"的位置，攻击面必须最小化。用户的实际图示（流程图、架构图）完全由绘图元素构成，严格白名单不影响表达力。

4. **存量不迁移**：本次修复只保证**以后**写的 SVG 能存能显。已发布文章里的 SVG 在当初保存时就被删了、数据库里没有；若保有原始 markdown，重新保存一次即可。

## 被否决的方案

- **`html: true` 但不消毒**：等于开放任意 HTML，任何能发文的人都能塞 `<script>`，读者打开文章即中招。不可接受。
- **只支持 ```svg 围栏代码块**（对标现有 mermaid 特判）：不开放任意 HTML，最安全；但要求用户改写所有存量内容，且 `<div>`/`<center>` 的排版能力丢失。为保留现有写作方式而否决。
- **允许上传 SVG 文件**：与"正文内联 SVG"是不同的需求路径（上传涉及格式白名单与存储侧审核），本次不做。

## 代价与约束

- 新增前端依赖（DOMPurify），且每次渲染多一道清洗；读者站与后台编辑器预览**共用同一条渲染管线**，两处一起生效，避免"读者站能显示、后台预览显示源码"的割裂。
- 后端放行 SVG 后，`MdToSafe` 的输出不再是"纯 markdown + 极少量 HTML"，下游（ES 投影、摘要生成、引用账本的 `<img src>` 解析）需要确认不受影响。
- 白名单是**逐步收紧容易、放宽难**：日后要加特效元素须重走本 ADR。
