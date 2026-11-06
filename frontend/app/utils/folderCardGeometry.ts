/**
 * 合集/项目文件夹卡的纯几何（grill 记录 §三 3.6，术语见 GLOSSARY「合集/项目文件夹卡」）。
 *
 * 形态是一条**比例公式**，不是一组写死的像素：网格列宽不变
 * （`minmax(min(100%, 20rem), 1fr)`），卡内的背板、封面、前盖都按
 * 「容器宽度 / 基宽」等比缩放，所以 20rem 的窄列与 30rem 的宽列是同一个形状。
 *
 * 基宽取参考形态的 321×270。参考实现是 321×270 的三张扇形，本票只有**一张**
 * 16:9 封面卡（三张扇形方案已被否，见 grill 记录 §三 3.6 与 §五）。
 *
 * 分工：
 *   - 这里只做算术与换算（纯函数，可单测）；
 *   - 样式表只消费 `folderCardStyleVars()` 的结果，不写死尺寸与角度。
 *
 * 为什么样式变量用百分比而不是 cqw / 容器查询：百分比的参照是浏览器算好的
 * 包含块与自身盒子，不需要容器查询单位（老浏览器会整体失效），也不需要
 * 在运行时测量列宽（那会让首帧闪一下）。
 */

/** 参考形态的基宽（px）：所有比例的换算基准，**不是**渲染尺寸。 */
export const FOLDER_CARD_BASE_WIDTH = 321;

/** 参考形态的基高（px）：同上，只用于比例换算。 */
export const FOLDER_CARD_BASE_HEIGHT = 270;

/** 封面卡固定 16:9（与 ArticleCover 同一个形状，票 204）。 */
const SHEET_ASPECT = 16 / 9;

/** 基宽下的几何（px）。出处见每行注释；改这里就等于改所有列宽下的形状。 */
const BASE = {
  /** 背板：左右各留 8，底边 264（舞台高 270 留 6 底白）。 */
  backLeft: 8,
  backTop: 12,
  backWidth: 305,
  backBottom: 264,
  /** 封面卡：236 宽（16:9 → 132.75 高），水平居中，顶部 46。 */
  sheetWidth: 236,
  sheetTop: 46,
  /** 前盖：比背板再收 2，底边与背板齐平。 */
  flapLeft: 10,
  flapWidth: 301,
  /** 封面露出上 2/3 —— 前盖顶边正好盖住下 1/3。 */
  visibleSheetFraction: 2 / 3,
  /** hover / 键盘聚焦时封面上抽的距离（基宽下）。 */
  lift: 30,
  /** 前盖常态前倾角（度）；展开时前倾到 -45°。 */
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

/** 基宽下的封面高度（px），用于把"30px 上抽"换算成占封面高度的比例。 */
function baseSheetHeightPx(): number {
  return folderCardSheetHeightPx(BASE.sheetWidth);
}

/** 基宽下的前盖顶边（px）：封面顶 + 2/3 封面高 = 露出上 2/3。 */
function baseFlapTopPx(): number {
  return BASE.sheetTop + baseSheetHeightPx() * BASE.visibleSheetFraction;
}

/** 值 ÷ 基准，换算成百分比字符串；两位小数、去掉多余的 0。 */
function percentOf(valuePx: number, basePx: number): string {
  const ratio = valuePx / basePx;
  return `${Number((ratio * 100).toFixed(2))}%`;
}

/** 样式表消费的那一份几何：全部相对容器（宽度 / 高度 / 角度），无像素尺寸。 */
export function folderCardStyleVars(): Record<string, string> {
  const flapTop = baseFlapTopPx();

  return {
    "--folder-stage-ratio": `${FOLDER_CARD_BASE_WIDTH} / ${FOLDER_CARD_BASE_HEIGHT}`,
    "--folder-back-left": percentOf(BASE.backLeft, FOLDER_CARD_BASE_WIDTH),
    "--folder-back-top": percentOf(BASE.backTop, FOLDER_CARD_BASE_HEIGHT),
    "--folder-back-width": percentOf(BASE.backWidth, FOLDER_CARD_BASE_WIDTH),
    "--folder-back-height": percentOf(
      BASE.backBottom - BASE.backTop,
      FOLDER_CARD_BASE_HEIGHT,
    ),
    "--folder-sheet-left": percentOf(
      (FOLDER_CARD_BASE_WIDTH - BASE.sheetWidth) / 2,
      FOLDER_CARD_BASE_WIDTH,
    ),
    "--folder-sheet-top": percentOf(BASE.sheetTop, FOLDER_CARD_BASE_HEIGHT),
    "--folder-sheet-width": percentOf(BASE.sheetWidth, FOLDER_CARD_BASE_WIDTH),
    "--folder-sheet-height": percentOf(baseSheetHeightPx(), FOLDER_CARD_BASE_HEIGHT),
    "--folder-sheet-lift": percentOf(
      -folderCardLiftPx(FOLDER_CARD_BASE_WIDTH),
      baseSheetHeightPx(),
    ),
    "--folder-sheet-tilt": `${folderCardSheetTiltDeg(true)}deg`,
    "--folder-flap-left": percentOf(BASE.flapLeft, FOLDER_CARD_BASE_WIDTH),
    "--folder-flap-top": percentOf(flapTop, FOLDER_CARD_BASE_HEIGHT),
    "--folder-flap-width": percentOf(BASE.flapWidth, FOLDER_CARD_BASE_WIDTH),
    "--folder-flap-height": percentOf(
      BASE.backBottom - flapTop,
      FOLDER_CARD_BASE_HEIGHT,
    ),
    "--folder-flap-tilt": `${folderCardFlapTiltDeg(false)}deg`,
    "--folder-flap-tilt-open": `${folderCardFlapTiltDeg(true)}deg`,
  };
}
