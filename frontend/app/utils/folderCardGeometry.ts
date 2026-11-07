/**
 * 合集/项目文件夹卡的纯几何（grill 记录 §三 3.6，术语见 GLOSSARY「合集/项目文件夹卡」）。
 *
 * 形态是**一组比例**，不是一组写死的像素：网格列宽不变
 * （`minmax(min(100%, 20rem), 1fr)`），卡内的背板、封面、前盖都按
 * 「容器宽度 / 基宽」等比缩放，所以 20rem 的窄列与 30rem 的宽列是同一个形状。
 *
 * 外形来自参考组件的**前盖轮廓**（`FOLDER_FLAP_PATH`，逐字照抄）：左上圆角、
 * 顶部凸舌、45° 斜切肩、右侧平顶、底部圆角。它就是「看起来像文件夹」的来源，
 * 其余层次（背板 / 封面 / 前盖）都围着这条轮廓排。
 *
 * 基宽取参考的 321×270 舞台、前盖 viewBox 321×241。参考里是三张扇形纸卡，
 * 本票只有**一张** 16:9 封面（三张扇形方案已被否，见 grill 记录 §三 3.6 与 §五）。
 *
 * 分工：
 *   - 这里只做算术与换算（纯函数，可单测）；
 *   - 样式表只消费 `folderCardStyleVars()` 的结果，不写死尺寸、角度与路径。
 *
 * 为什么样式变量用百分比而不是 cqw / 容器查询：百分比的参照是浏览器算好的
 * 包含块与自身盒子，不需要容器查询单位（老浏览器会整体失效），也不需要
 * 在运行时测量列宽（那会让首帧闪一下）。
 */

/** 参考形态的基宽（px）：所有比例的换算基准，**不是**渲染尺寸。 */
export const FOLDER_CARD_BASE_WIDTH = 321;

/** 参考形态的基高（px）：同上，只用于比例换算。**比参考的 270 矮**——
 *  舞台压扁之后，文件夹只比封面宽一点点（用户 2026-11-07 的返工口径）。 */
export const FOLDER_CARD_BASE_HEIGHT = 221;

/** 前盖（flap）在基宽下的尺寸：参考组件的 viewBox 是 321×241。 */
export const FOLDER_FLAP_WIDTH = 321;
export const FOLDER_FLAP_HEIGHT = 241;

/** 前盖轮廓的圆角半径（与下面这条 path 里的 25 同源，背板跟着它走）。 */
export const FOLDER_FLAP_RADIUS = 25;

/**
 * 凸舌肩线的 y（前盖自身坐标）：右侧平顶从这条线开始。
 *
 * 数值取自下面 path 里第一个水平段 `H296` 的 y —— 前盖盖住封面下沿的位置
 * 就是这条线，所以「封面露出上 2/3」是按它算的，不是按前盖顶边算的。
 */
export const FOLDER_FLAP_SHOULDER_Y = 41.5404;

/**
 * 前盖轮廓：**逐字**来自参考组件（左上圆角 → 凸舌顶边 → 45° 斜切肩 →
 * 右侧平顶 → 直边 → 底部圆角）。改这里之前先想清楚：这是"文件夹"三个字。
 */
export const FOLDER_FLAP_PATH =
  "M0 25C0 11.1929 11.1929 0 25 0H136.084C143.044 0 149.689 2.90139 154.42 8.00608L178.08 33.5343C182.811 38.639 189.456 41.5404 196.416 41.5404H296C309.807 41.5404 321 52.7333 321 66.5404V216C321 229.807 309.807 241 296 241H25C11.1929 241 0 229.807 0 216V25Z";

/** 封面卡固定 16:9（与 ArticleCover 同一个形状，票 204）。 */
const SHEET_ASPECT = 16 / 9;

/** 基宽下的几何（px）。出处见每行注释；改这里就等于改所有列宽下的形状。 */
const BASE = {
  /** 背板：铺满宽度，顶边 4（比前盖低，露一条后沿），底边与舞台齐平。 */
  backTop: 4,
  backBottom: 221,
  /**
   * 前盖：321 宽压满，只占舞台下部的 97 高（底边 215，留 6 当背板的后沿）。
   *
   * 顶边 **不再**是参考的 23 —— 参考里前盖是整张前脸（241 高），本票要「文件夹只比
   * 封面宽一点点」，所以前盖收成一道前唇，顶边压到封面下 30% 处。
   */
  flapTop: 118,
  flapBottom: 215,
  /**
   * 封面卡：283 宽（16:9 → 159.19 高），水平居中。
   * 舞台宽 321，所以文件夹只比封面宽 13%——这就是"稍宽一点点"。
   */
  sheetWidth: 283,
  /** 封面顶边：留 6 的空隙，不顶到舞台边框。 */
  sheetTop: 6,
  /** hover / 键盘聚焦时封面上抽的距离（基宽下）。 */
  lift: 24,
  /** 前盖常态前倾角（度）；展开时前倾到 -45°（铰链在下沿）。 */
  flapTilt: -15,
  flapTiltOpen: -45,
  /** 封面展开时的旋转角（度）。 */
  sheetTilt: 6,
} as const;

/** 缩放系数 = 容器宽度 / 基宽；宽度不可用时按 0（调用方不得据此画几何）。 */
export function folderCardScale(containerWidthPx: number): number {
  if (!Number.isFinite(containerWidthPx) || containerWidthPx <= 0) return 0;
  return containerWidthPx / FOLDER_CARD_BASE_WIDTH;
}

/** 16:9 封面的高度（由宽度决定，任何列宽下形状不变）。 */
export function folderCardSheetHeightPx(sheetWidthPx: number): number {
  if (!Number.isFinite(sheetWidthPx) || sheetWidthPx <= 0) return 0;
  return sheetWidthPx / SHEET_ASPECT;
}

/**
 * 抽卡距离（px）：基宽下 30px，等比缩放。
 *
 * 只回答"展开时抽多远"这一个问题。**未展开**与**减少动态**的归零不在这里分叉：
 * 前者由 CSS 的 `:hover` / `:focus-visible` 决定（不展开就不套那份位移），
 * 后者由 `prefers-reduced-motion` 的兜底把位移归零。算式里带上这两个开关，
 * 生产代码一次都不会传——那只是把 CSS 已经管好的事再抄一份到 JS 里。
 */
export function folderCardLiftPx(containerWidthPx: number): number {
  return BASE.lift * folderCardScale(containerWidthPx);
}

/** 前盖倾角（度）：常态 -15°，hover / 聚焦 -45°。 */
export function folderCardFlapTiltDeg(revealed: boolean): number {
  return revealed ? BASE.flapTiltOpen : BASE.flapTilt;
}

/** 封面旋转角（度）：常态 0°，hover / 聚焦 6°。 */
export function folderCardSheetTiltDeg(revealed: boolean): number {
  return revealed ? BASE.sheetTilt : 0;
}

/** 前盖斜切肩在舞台坐标里的 y（px）：封面「露出上 2/3」就是拿它当遮盖线。 */
export function folderFlapShoulderY(): number {
  return BASE.flapTop + FOLDER_FLAP_SHOULDER_Y;
}

/** 封面顶边（px）：固定留 6 的空隙，不顶到舞台边框。 */
export function folderSheetTopPx(): number {
  return BASE.sheetTop;
}

/**
 * 前盖的 `mask-image`：参考组件的轮廓当遮罩（`preserveAspectRatio="none"`，
 * 所以任意列宽下都铺满元素、形状不变）。
 *
 * 为什么用 mask 而不是 `clip-path: path()`：`path()` 只认像素、不随列宽缩放，
 * 等于把 321×241 钉死；data-URI 的 SVG 有自己的 viewBox，跟着元素盒子走。
 * 遮罩里的 `fill="white"` 只需要 alpha，不参与配色。
 */
export function folderFlapMaskImage(): string {
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${FOLDER_FLAP_WIDTH} ${FOLDER_FLAP_HEIGHT}" preserveAspectRatio="none">` +
    `<path d="${FOLDER_FLAP_PATH}" fill="white"/></svg>`;
  // 括号与引号必须一起编掉：这个值要穿过 CSS 自定义属性，而自定义属性又常常
  // 写在 `style="…"` 属性里——留一个裸括号就可能把外层的引号/函数提前收掉，
  // 结果是 mask 解析成空、整个前盖凭空消失（正是这里踩过的坑）。
  const encoded = encodeURIComponent(svg).replace(
    /['()]/gu,
    (char) => `%${char.charCodeAt(0).toString(16).toUpperCase()}`,
  );
  return `url('data:image/svg+xml,${encoded}')`;
}

/** 值 ÷ 基准，换算成百分比字符串；两位小数、去掉多余的 0。 */
function percentOf(valuePx: number, basePx: number): string {
  const ratio = valuePx / basePx;
  return `${Number((ratio * 100).toFixed(2))}%`;
}

/** 样式表消费的那一份几何：全部相对容器（宽度 / 高度 / 角度），无像素尺寸。 */
export function folderCardStyleVars(): Record<string, string> {
  const sheetHeight = folderCardSheetHeightPx(BASE.sheetWidth);
  const sheetTop = folderSheetTopPx();

  return {
    "--folder-stage-ratio": `${FOLDER_CARD_BASE_WIDTH} / ${FOLDER_CARD_BASE_HEIGHT}`,
    "--folder-back-top": percentOf(BASE.backTop, FOLDER_CARD_BASE_HEIGHT),
    "--folder-back-height": percentOf(
      BASE.backBottom - BASE.backTop,
      FOLDER_CARD_BASE_HEIGHT,
    ),
    "--folder-sheet-left": percentOf(
      (FOLDER_CARD_BASE_WIDTH - BASE.sheetWidth) / 2,
      FOLDER_CARD_BASE_WIDTH,
    ),
    "--folder-sheet-top": percentOf(sheetTop, FOLDER_CARD_BASE_HEIGHT),
    "--folder-sheet-width": percentOf(BASE.sheetWidth, FOLDER_CARD_BASE_WIDTH),
    "--folder-sheet-height": percentOf(sheetHeight, FOLDER_CARD_BASE_HEIGHT),
    "--folder-sheet-lift": percentOf(
      -folderCardLiftPx(FOLDER_CARD_BASE_WIDTH),
      sheetHeight,
    ),
    "--folder-sheet-tilt": `${folderCardSheetTiltDeg(true)}deg`,
    "--folder-flap-top": percentOf(BASE.flapTop, FOLDER_CARD_BASE_HEIGHT),
    "--folder-flap-height": percentOf(
      BASE.flapBottom - BASE.flapTop,
      FOLDER_CARD_BASE_HEIGHT,
    ),
    "--folder-flap-tilt": `${folderCardFlapTiltDeg(false)}deg`,
    "--folder-flap-tilt-open": `${folderCardFlapTiltDeg(true)}deg`,
    "--folder-flap-mask": folderFlapMaskImage(),
  };
}
