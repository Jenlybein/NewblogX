/**
 * 首屏「哪些元素该被装饰让开」的统一收集器。
 *
 * 像素尾迹要按字符位置躲开元素；指针墨流要把元素当成流场里的实心墙。
 * 两边要是各写一套筛选，久了两套边界一定对不上 —— 所以判据只在这里。
 *
 * 判据分两层：
 *   1. 几何：面积超过宿主 60% 的容器排除（否则整个首屏都会变成墙），
 *      小于 4×4 的碎片也排除（那些基本都是伪元素级别的装饰）；
 *   2. 语义：只留「叶子块」或「自己有看得见的墨」的块。
 *      刻度条是后者 —— 数字在 <li> 里，那条发丝线是 <ul> 自己的 border-top，
 *      只按叶子筛就会漏掉线，字压线看着才叫脏。
 */

/** 一个会挡字的元素外框（相对宿主坐标，已含它自己的圆角半径）。 */
export interface OccluderBox {
  x: number;
  y: number;
  w: number;
  h: number;
  radius: number;
}

/**
 * 点是否落在「外拓 padding 后的圆角矩形」内。
 *
 * 分成两步是为了让圆角真的参与判断：先按外拓矩形快速排除，再单独判四角的
 * 圆弧之外 —— 元素是圆角卡片时，装饰不会在圆角外那一小块被白白吞掉。
 */
export function hitsRoundedBox(x: number, y: number, box: OccluderBox, padding: number): boolean {
  const left = box.x - padding;
  const top = box.y - padding;
  const right = box.x + box.w + padding;
  const bottom = box.y + box.h + padding;
  if (x <= left || x >= right || y <= top || y >= bottom) return false;
  const radius = Math.min(box.radius + padding, (right - left) / 2, (bottom - top) / 2);
  if (radius <= 0) return true;
  const cornerX = x < left + radius ? left + radius : x > right - radius ? right - radius : x;
  const cornerY = y < top + radius ? top + radius : y > bottom - radius ? bottom - radius : y;
  const dx = x - cornerX;
  const dy = y - cornerY;
  return dx * dx + dy * dy <= radius * radius;
}

/**
 * 一个计算后的颜色是否"看不见"（透明 / alpha 为 0）。
 *
 * 不写字面色值：只解析 `rgb()/rgba()` 里的第四个数，够用且不会把 token 语义
 * 抄进组件（门禁禁止组件里出现色值字面量）。
 */
function isTransparentColor(value: string): boolean {
  if (value === "transparent" || value === "") return true;
  const match = value.match(/^rgba?\(([^)]+)\)$/);
  if (!match) return false;
  const parts = (match[1] ?? "").split(",").map((part) => part.trim());
  return parts.length === 4 && Number.parseFloat(parts[3] ?? "1") === 0;
}

/** 元素自己有没有"看得见的墨"：描边、底色、阴影任一条就算。 */
export function hasVisibleInk(
  style: Pick<
    CSSStyleDeclaration,
    | "borderTopWidth"
    | "borderRightWidth"
    | "borderBottomWidth"
    | "borderLeftWidth"
    | "backgroundColor"
    | "boxShadow"
  >,
): boolean {
  const borderWidths = [
    style.borderTopWidth,
    style.borderRightWidth,
    style.borderBottomWidth,
    style.borderLeftWidth,
  ];
  const hasBorder = borderWidths.some((width) => Number.parseFloat(width) > 0);
  const hasBackground = !isTransparentColor(style.backgroundColor);
  return hasBorder || hasBackground || style.boxShadow !== "none";
}

export interface CollectOccludersOptions {
  /** 小于这个边长（px）的碎片直接忽略。 */
  minSize?: number;
  /** 面积超过宿主这个比例的容器直接忽略（首屏版式容器）。 */
  maxAreaRatio?: number;
  /**
   * 命中选择器的元素及其子树整个跳过。
   *
   * 给装饰层留的出口：字标、刻度标尺、指针这些"本来就该浮在墨上面"的东西，
   * 声明了 `data-ink-pass` 就不再是墙 —— 墨直接穿过去。
   */
  skipSelector?: string;
}

/**
 * 扫一遍宿主里"会挡装饰"的元素，返回相对宿主的圆角外框。
 *
 * `exclude` 是装饰自己（画布）—— 它自己的祖先链不算遮挡物，否则整屏都是墙。
 */
export function collectOccluders(
  host: Element | null,
  exclude: Element | null,
  options: CollectOccludersOptions = {},
): OccluderBox[] {
  if (!host) return [];
  const minSize = options.minSize ?? 4;
  const maxAreaRatio = options.maxAreaRatio ?? 0.6;
  const skipSelector = options.skipSelector ?? "";
  const base = host.getBoundingClientRect();
  const hostArea = Math.max(1, base.width * base.height);
  const candidates: Array<{
    el: Element;
    ink: boolean;
    boxes: OccluderBox[];
    /** 元素自身有没有"画了东西"：墨、图片、或者本身就是文字。 */
    paint: boolean;
    /** boxes 是不是"逐字框"（而不是整块外框）。 */
    isGlyphs: boolean;
    /** 元素的整块外框（判定"装饰压着字"时用）。 */
    rect: OccluderBox;
  }> = [];

  for (const el of host.querySelectorAll<HTMLElement>("*")) {
    if (exclude && (el === exclude || exclude.contains(el) || el.contains(exclude))) continue;
    if (skipSelector && el.closest(skipSelector)) continue;
    const rect = el.getBoundingClientRect();
    if (rect.width < minSize || rect.height < minSize) continue;
    if (rect.width * rect.height > hostArea * maxAreaRatio) continue;
    const style = getComputedStyle(el);
    if (style.display === "none" || style.visibility === "hidden" || Number(style.opacity) === 0) {
      continue;
    }
    const box: OccluderBox = {
      x: rect.left - base.left,
      y: rect.top - base.top,
      w: rect.width,
      h: rect.height,
      radius: Number.parseFloat(style.borderTopLeftRadius) || 0,
    };
    const ink = hasVisibleInk(style);
    const text = el.textContent?.trim() ?? "";
    // 纯文字块取**字形所在的框**，不要整块元素的外框：
    // 标题是块级元素，外框横跨整行，按外框算会在版面里压出一条贯穿的黑带。
    const glyphs = ink || !text ? null : textRunBoxes(el, base, minSize);
    candidates.push({
      el,
      ink,
      paint: ink || Boolean(text) || el.querySelector("img, canvas, video, picture") !== null,
      isGlyphs: glyphs !== null,
      rect: box,
      boxes: glyphs ?? [box],
    });
  }

  // 字框：既是墙体，也是"装饰不该盖住字"的参照。
  const glyphBoxes = candidates
    .filter((candidate) => candidate.isGlyphs)
    .flatMap((candidate) => candidate.boxes);

  /**
   * 装饰层（标题的雾光、玻璃碎片、渐变压片…）会连着字一起被当成墙，
   * 于是"本该从字缝里穿过去的墨"被整块挡住 —— 观感上就是没碰到字。
   * 判据：这一块自己的面积里，被字框占了 15% 以上 → 它是压着字的装饰，让位给字。
   */
  const isTextBackdrop = (box: OccluderBox): boolean => {
    if (glyphBoxes.length === 0) return false;
    const area = Math.max(1, box.w * box.h);
    let covered = 0;
    for (const glyph of glyphBoxes) {
      const w = Math.min(box.x + box.w, glyph.x + glyph.w) - Math.max(box.x, glyph.x);
      const h = Math.min(box.y + box.h, glyph.y + glyph.h) - Math.max(box.y, glyph.y);
      if (w > 0 && h > 0) covered += w * h;
    }
    return covered / area > 0.15;
  };

  // 叶子块留下；有墨（描边 / 底色 / 阴影）的父块也留下 —— 它自己就是一条看得见的版式。
  return candidates
    .filter((candidate) => {
      // 压着字的装饰（雾光、玻璃碎片、渐变压片）让位给字：它自己不该是墙。
      if (!candidate.el.hasAttribute("data-glass-text") && isTextBackdrop(candidate.rect)) {
        return false;
      }
      // 既不画东西、也没有可见子块的叶子（纯晕光 / 纯布局壳）不算遮挡物。
      if (!candidate.paint && candidate.boxes.length === 1 && candidate.boxes[0] === candidate.rect) {
        if (!candidates.some((other) => other.el !== candidate.el && candidate.el.contains(other.el))) {
          return false;
        }
      }
      return (
        candidate.ink ||
        !candidates.some((other) => other.el !== candidate.el && candidate.el.contains(other.el))
      );
    })
    .flatMap((candidate) => candidate.boxes);
}

/**
 * 纯文字元素 → 它自己那几个**字框**（相对宿主）。
 *
 * `Range.getClientRects()` 给的是文字实际占据的矩形，而不是块级元素撑满整行的外框。
 * 短文本按**单个字**切（标题就变成一字一框，装饰只躲字本身、可以从字间穿过去）；
 * 长文本退回整行，避免几十个 Range 把每帧的扫描拖慢。
 *
 * 拿不到（空元素 / 隐藏）时返回 null，调用方退回元素外框。
 */
function textRunBoxes(el: Element, base: DOMRect, minSize: number): OccluderBox[] | null {
  const text = el.textContent?.trim() ?? "";
  if (!text) return null;
  const doc = el.ownerDocument;
  const range = doc.createRange();
  const boxes: OccluderBox[] = [];
  const push = (rect: DOMRect) => {
    if (rect.width < minSize || rect.height < minSize) return;
    boxes.push({
      x: rect.left - base.left,
      y: rect.top - base.top,
      w: rect.width,
      h: rect.height,
      radius: 0,
    });
  };

  const perChar = text.length <= PER_CHAR_LIMIT && el.childNodes.length > 0;
  if (perChar) {
    const walker = doc.createTreeWalker(el, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      const value = node.nodeValue ?? "";
      for (let i = 0; i < value.length; i += 1) {
        if (!value[i]?.trim()) continue;
        range.setStart(node, i);
        range.setEnd(node, i + 1);
        for (const rect of Array.from(range.getClientRects())) push(rect);
      }
    }
  }
  if (boxes.length > 0) return boxes;

  range.selectNodeContents(el);
  const rects = Array.from(range.getClientRects());
  for (const rect of rects) push(rect);
  return boxes.length > 0 ? boxes : null;
}

/** 超过这个字数的文字块按整行处理（逐字切 Range 的开销不值当）。 */
const PER_CHAR_LIMIT = 64;
