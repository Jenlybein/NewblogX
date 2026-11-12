/**
 * 搜索结果里的高亮片段。
 *
 * 后端（ES 的 highlight）把命中的词包在 `<em>` 里返回，前端**不能**直接
 * `v-html`：标题来自用户内容，那等于把 XSS 装回去。也不能原样渲染 ——
 * 那样页面上会直接出现 `<em>深</em>度<em>学</em>习` 这种未转义字符
 * （2026-11-12 线上反馈）。
 *
 * 所以在这里把它切成"纯文本 + 命中标记"的片段，交给模板逐个渲染成 <mark>：
 * 既不解析 HTML，也不丢掉高亮。
 */
export interface HighlightSegment {
  text: string;
  /** true 表示这段是命中词，模板里用它包一层强调。 */
  hit: boolean;
}

/** 后端可能返回裸 `<em>`，也可能返回转义过的 `&lt;em&gt;`，两种都认。 */
const OPEN_TAG = /<em>|&lt;em&gt;/;
const CLOSE_TAG = /<\/em>|&lt;\/em&gt;/;

/** 把带 `<em>` 标记的字符串切成片段；没有标记时返回单段。 */
export function splitHighlight(input: string | null | undefined): HighlightSegment[] {
  const raw = (input ?? "").toString();
  if (!raw) return [{ text: "", hit: false }];

  const segments: HighlightSegment[] = [];
  let rest = raw;

  // 用循环而不是正则 split：只有成对的 <em>…</em> 才算命中，
  // 落单的开/闭标签按普通文本处理（否则一个残缺标签会把后半段整段吞掉）。
  for (;;) {
    const open = rest.search(OPEN_TAG);
    if (open < 0) break;

    const openMatch = rest.slice(open).match(OPEN_TAG);
    const openLength = openMatch ? openMatch[0].length : 0;
    const afterOpen = rest.slice(open + openLength);
    const close = afterOpen.search(CLOSE_TAG);
    if (close < 0) break;

    const closeMatch = afterOpen.slice(close).match(CLOSE_TAG);
    const closeLength = closeMatch ? closeMatch[0].length : 0;

    if (open > 0) segments.push({ text: rest.slice(0, open), hit: false });
    segments.push({ text: afterOpen.slice(0, close), hit: true });
    rest = afterOpen.slice(close + closeLength);
  }

  if (rest) segments.push({ text: rest, hit: false });
  return segments.length ? segments : [{ text: "", hit: false }];
}

/** 只想要纯文本时用它（例如塞进 aria-label / title）。 */
export function stripHighlight(input: string | null | undefined): string {
  return splitHighlight(input)
    .map(segment => segment.text)
    .join("");
}
