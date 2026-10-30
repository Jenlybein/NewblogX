/**
 * 提示块（`::: tip` / `::: note` / `::: warning` / `::: danger`）的切分规则。
 *
 * spec 要求"代码块、表格、数学公式、提示块"与新配色一致；markdown-it 本身不支持
 * 容器语法，也不想为这一个块引入新依赖，所以用最小实现的纯函数切分：
 *
 *   把正文按提示块切成若干段（普通 markdown / 提示块），渲染时分别处理。
 *
 * 规则故意保守：只认四种已知语气、必须显式闭合；未知语气或未闭合的标记按原文
 * 输出（不能因为一个 `:::` 把后面整篇正文吞进提示块）。
 */

export type NoticeTone = "tip" | "note" | "warning" | "danger";

export const NOTICE_TONES: NoticeTone[] = ["tip", "note", "warning", "danger"];

export type NoticeSegment =
  | { type: "markdown"; source: string }
  | { type: "notice"; tone: NoticeTone; title: string; source: string };

const NOTICE_OPEN_RE = /^:::\s*([a-z]+)\s*(.*)$/iu;
const NOTICE_CLOSE_RE = /^:::\s*$/u;

function isNoticeTone(value: string): value is NoticeTone {
  return (NOTICE_TONES as string[]).includes(value);
}

 /** 把正文切分成普通 markdown 段与提示块段。 */
export function splitNoticeSegments(source: string): NoticeSegment[] {
  const lines = source.split(/\r?\n/u);
  const segments: NoticeSegment[] = [];
  let markdownBuffer: string[] = [];

  const flushMarkdown = () => {
    if (!markdownBuffer.length) {
      return;
    }
    const text = markdownBuffer.join("\n");
    if (text.trim()) {
      segments.push({ type: "markdown", source: text });
    }
    markdownBuffer = [];
  };

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index] ?? "";
    const openMatch = NOTICE_OPEN_RE.exec(line);
    const tone = openMatch?.[1]?.toLowerCase() ?? "";
    if (!openMatch || !isNoticeTone(tone)) {
      markdownBuffer.push(line);
      continue;
    }

    // 找闭合行；找不到就当普通文本（含开标记那一行原样保留）。
    let closeIndex = index + 1;
    while (closeIndex < lines.length && !NOTICE_CLOSE_RE.test(lines[closeIndex] ?? "")) {
      closeIndex += 1;
    }
    if (closeIndex >= lines.length) {
      markdownBuffer.push(line);
      continue;
    }

    flushMarkdown();
    segments.push({
      type: "notice",
      tone,
      title: (openMatch[2] ?? "").trim(),
      source: lines.slice(index + 1, closeIndex).join("\n").trim(),
    });
    index = closeIndex;
  }

  flushMarkdown();
  return segments;
}
