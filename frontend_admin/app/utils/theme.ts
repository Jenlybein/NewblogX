/**
 * 后台主题层：把 `tokens.css` 的**语义层**翻译成 Naive UI 的 themeOverrides。
 *
 * 为什么需要这个模块（而不是在 app.vue 里写一串十六进制）：
 *
 *   1. **颜色只有一个来源**（ADR-0012 约束 4）。这里直接解析 `--color-*` 语义层，
 *      不再像旧版那样先读原始层 hex 再自己拼一遍 —— 那等于给颜色留了第二套来源。
 *   2. **Naive 不接受 CSS 变量当颜色**。它的 `self()` 会用 seemly 的
 *      `changeColor()` / `composite()` 对传入颜色做运算（Tag、Alert、DataTable、
 *      Menu 等都会），传 `var(--color-x)` 会抛 "Invalid color value"。所以这里把
 *      `var(--x)` 与 `rgb(var(--x-rgb) / α)` 展开成真实色值：hex 或 rgba。
 *   3. **暗色是同一组 token 的第二份取值**。`parseThemePalette(css, "dark")` 读
 *      `.dark` 块，`app.vue` 按用户选择换一份 palette 即可 —— 没有第二套颜色表。
 *   4. **可测**。取值是纯函数（解析 + 三元组派生 + 覆盖对象组装），单测直接断言
 *      "主色是墨色、危险字用派生暗橙、圆角来自阶梯、暗色对比度达标"。
 *
 * 透明度从哪来：所有 `rgba()` 都由 `--*-rgb` 三元组派生（`withTripletAlpha`），
 * 比例集中在本文件的 `ALPHA` 命名表里，调用处不写魔法数字。源码里不许出现
 * `rgba(数字…)` 常量，也不许出现裸 hex —— 有单测钉住。
 */
import type { GlobalThemeOverrides } from "naive-ui";

/** 语义层 token：键名 → `tokens.css` 里的变量名。组件只消费这一层。 */
export const THEME_TOKEN_VARS = {
  ink: "--color-ink",
  inkMuted: "--color-ink-muted",
  inkFaint: "--color-ink-faint",
  surface: "--color-surface",
  surfaceRaised: "--color-surface-raised",
  surfaceGlass: "--color-surface-glass",
  bgPage: "--color-bg-page",
  bgSubtle: "--color-bg-subtle",
  line: "--color-line",
  lineStrong: "--color-line-strong",
  lineFaint: "--color-line-faint",
  link: "--color-link",
  focusSoft: "--color-focus-soft",
  highlight: "--color-highlight",
  danger: "--color-danger",
  dangerInk: "--color-danger-ink",
  dangerSoft: "--color-danger-soft",
  dangerLine: "--color-danger-line",
  onStrong: "--color-on-strong",
  onStrongMuted: "--color-on-strong-muted",
  onStrongLine: "--color-on-strong-line",
  onStrongSoft: "--color-on-strong-soft",
  onDanger: "--color-on-danger",
  onInverse: "--color-on-inverse",
  onInverseMuted: "--color-on-inverse-muted",
  surfaceInverse: "--color-surface-inverse",
  shadow: "--color-shadow",
} as const;

/** 三元组基底：唯一允许派生透明度的来源（暗色块里换第二份通道值）。 */
export const THEME_TRIPLET_VARS = {
  graphite: "--graphite-rgb",
  interactive: "--interactive-rgb",
  yellow: "--jelly-yellow-rgb",
  coral: "--jelly-coral-rgb",
  cream: "--jelly-cream-rgb",
  onStrong: "--on-strong-rgb",
} as const;

export type ThemeTone = keyof typeof THEME_TOKEN_VARS;
export type ThemePalette = Record<ThemeTone, string>;
export type ThemeTripletTone = keyof typeof THEME_TRIPLET_VARS;
export type ThemeTriplets = Record<ThemeTripletTone, string>;
export type ThemeMode = "light" | "dark";

/** 圆角五档（与 tokens.css 同一份取值）。 */
export interface ThemeScale {
  sm: string;
  md: string;
  lg: string;
  xl: string;
  pill: string;
}

const RADIUS_VARS: Record<keyof ThemeScale, string> = {
  sm: "--radius-sm",
  md: "--radius-md",
  lg: "--radius-lg",
  xl: "--radius-xl",
  pill: "--radius-pill",
};

/**
 * 透明度档位：Naive 需要一批 token 层没有单列的淡色（悬停底、斑马纹、滚动条…）。
 * 集中成命名表，调用处不再出现数字；改一处就是全站改一处。
 */
export const ALPHA = {
  placeholder: 0.7,
  tableHover: 0.04,
  tableStriped: 0.02,
  hover: 0.06,
  pressed: 0.12,
  avatar: 0.12,
  code: 0.06,
  button2: 0.06,
  button2Hover: 0.1,
  button2Pressed: 0.15,
  scrollbar: 0.25,
  scrollbarHover: 0.4,
  progressRail: 0.08,
  rail: 0.12,
} as const;

/** 英文正文字体 Inter + 中文自托管字体 + 系统中文 fallback（顺序有意义）。 */
export const FONT_STACK =
  '"Inter", "Inter Fallback", "Noto Sans SC", "Noto Sans SC Fallback", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif';

export const FONT_MONO =
  '"Cascadia Code", "JetBrains Mono", "Sofia Sans Mono", ui-monospace, monospace';

const DECLARATION_RE = /(--[a-z0-9-]+)\s*:\s*([^;]+);/giu;
/** `var(--x)` 直接引用。 */
const VAR_REFERENCE_RE = /^var\(\s*(--[a-z0-9-]+)\s*\)$/u;
/** 三元组派生写法：`rgb(var(--jelly-*-rgb) / α)`。 */
const TRIPLET_REFERENCE_RE = /^rgb\(\s*var\(\s*(--[a-z0-9-]+)\s*\)\s*\/\s*([\d.]+)\s*\)$/u;
/** 三元组的真实取值：空格分隔的三通道。 */
const TRIPLET_CHANNELS_RE = /^(\d{1,3})\s+(\d{1,3})\s+(\d{1,3})$/u;

type TokenBlocks = { light: Record<string, string>; dark: Record<string, string> };

function readDeclarations(cssText: string, selectorPattern: RegExp): Record<string, string> {
  const block = selectorPattern.exec(cssText)?.[1] ?? "";
  const declarations: Record<string, string> = {};

  for (const match of block.matchAll(DECLARATION_RE)) {
    declarations[match[1]!.trim()] = match[2]!.replace(/\s+/gu, " ").trim();
  }

  return declarations;
}

function parseTokenBlocks(cssText: string): TokenBlocks {
  return {
    light: readDeclarations(cssText, /:root\s*\{([\s\S]*?)\n\s*\}/u),
    dark: readDeclarations(cssText, /(?:\.dark|\.theme-dark)[^\n{]*\{([\s\S]*?)\n\s*\}/u),
  };
}

/** 解析一个自定义属性的最终值（支持 `var(--x)` 递归与三元组展开）。 */
function resolveTokenValue(blocks: TokenBlocks, mode: ThemeMode, name: string): string {
  const raw = blocks[mode][name] ?? blocks.light[name];
  if (!raw) throw new Error(`tokens.css 缺 token ${name}`);

  const reference = VAR_REFERENCE_RE.exec(raw);
  if (reference) return resolveTokenValue(blocks, mode, reference[1]!);

  const triplet = TRIPLET_REFERENCE_RE.exec(raw);
  if (triplet) {
    const channels = TRIPLET_CHANNELS_RE.exec(resolveTokenValue(blocks, mode, triplet[1]!));
    if (!channels) throw new Error(`token ${triplet[1]} 不是 "R G B" 三元组基底`);

    return `rgba(${channels[1]}, ${channels[2]}, ${channels[3]}, ${triplet[2]})`;
  }

  return raw;
}

/** 从 tokens.css 解析语义层；缺任何一个 token 都抛错（不静默回落 Naive 默认青绿）。 */
export function parseThemePalette(cssText: string, mode: ThemeMode = "light"): ThemePalette {
  const blocks = parseTokenBlocks(cssText);
  const palette = {} as ThemePalette;

  for (const [tone, tokenVar] of Object.entries(THEME_TOKEN_VARS)) {
    try {
      palette[tone as ThemeTone] = resolveTokenValue(blocks, mode, tokenVar);
    } catch {
      throw new Error(
        `tokens.css 缺 token ${tokenVar}（Naive 主题需要真实色值，不能用 CSS 变量占位）`,
      );
    }
  }

  return palette;
}

/** 解析三元组基底（暗色块里石墨阶 / 功能蓝 / 实心面内容色换第二份取值）。 */
export function parseThemeTriplets(cssText: string, mode: ThemeMode = "light"): ThemeTriplets {
  const blocks = parseTokenBlocks(cssText);
  const triplets = {} as ThemeTriplets;

  for (const [tone, tokenVar] of Object.entries(THEME_TRIPLET_VARS)) {
    let resolved: string;
    try {
      resolved = resolveTokenValue(blocks, mode, tokenVar);
    } catch {
      throw new Error(`tokens.css 缺三元组 ${tokenVar}（带透明度的颜色只能由它派生）`);
    }
    const channels = TRIPLET_CHANNELS_RE.exec(resolved);
    if (!channels) {
      throw new Error(`tokens.css 缺三元组 ${tokenVar}（带透明度的颜色只能由它派生）`);
    }
    triplets[tone as ThemeTripletTone] = `${channels[1]} ${channels[2]} ${channels[3]}`;
  }

  return triplets;
}

/** 从 `:root` 解析五档圆角；缺一档直接抛错（不允许静默归零）。 */
export function parseThemeScale(cssText: string): ThemeScale {
  const root = readDeclarations(cssText, /:root\s*\{([\s\S]*?)\n\s*\}/u);
  const scale = {} as ThemeScale;

  for (const [tier, tokenVar] of Object.entries(RADIUS_VARS)) {
    const value = root[tokenVar];
    if (!value) throw new Error(`tokens.css 缺圆角 token ${tokenVar}（Naive 主题需要真实圆角值）`);
    scale[tier as keyof ThemeScale] = value;
  }

  return scale;
}

/** 三元组透明度派生：`rgba(r, g, b, α)`，通道值来自 token 层。 */
export function withTripletAlpha(triplet: string, alpha: number): string {
  const channels = TRIPLET_CHANNELS_RE.exec(triplet.trim());
  if (!channels) throw new Error(`不是合法的 R G B 三元组：${triplet}`);
  const clamped = Math.min(1, Math.max(0, alpha));

  return `rgba(${channels[1]}, ${channels[2]}, ${channels[3]}, ${clamped})`;
}

function channelLuminance(value: number): number {
  const normalized = value / 255;

  return normalized <= 0.04045 ? normalized / 12.92 : ((normalized + 0.055) / 1.055) ** 2.4;
}

/** WCAG 2.x 相对亮度。 */
export function relativeLuminance(hex: string): number {
  const raw = hex.trim().replace(/^#/u, "");

  return (
    0.2126 * channelLuminance(Number.parseInt(raw.slice(0, 2), 16)) +
    0.7152 * channelLuminance(Number.parseInt(raw.slice(2, 4), 16)) +
    0.0722 * channelLuminance(Number.parseInt(raw.slice(4, 6), 16))
  );
}

/** WCAG 2.x 对比度（1..21）。AA 正文门槛 4.5，UI 边界门槛 3。 */
export function contrastRatio(first: string, second: string): number {
  const lighter = Math.max(relativeLuminance(first), relativeLuminance(second));
  const darker = Math.min(relativeLuminance(first), relativeLuminance(second));

  return (lighter + 0.05) / (darker + 0.05);
}

/**
 * 生成后台的 Naive 主题覆盖。
 *
 * 颜色策略（全部来自语义 token，不出现第六个色相、也不出现第二套色值）：
 *   - primary   = 墨色（CTA 是墨水实心胶囊，暗色下反转为浅墨填充）
 *   - info      = 功能蓝（只用于状态语义；hover/pressed 退回中性色，不派生蓝填充）
 *   - success   = 墨色（五色锁死，不引入绿）
 *   - warning   = 包豪斯黄且**只做底**：文字/图标一律换墨色（黄永不做字色）
 *   - error     = 珊瑚橙；填充上的字用 `--color-on-danger`（珊瑚橙对墨色 6:1）
 *   - 中性面/线 = 语义层（纸面 / 抬升面 / 次底，结构线三档）
 *   - 圆角      = `--radius-sm/md/lg/xl/pill` 五档
 *   - 透明度    = `--*-rgb` 三元组 + `ALPHA` 命名表
 */
export function buildNaiveThemeOverrides(
  palette: ThemePalette,
  triplets: ThemeTriplets,
  scale: ThemeScale,
): GlobalThemeOverrides {
  const warningFill = withTripletAlpha(triplets.yellow, 1);
  /** 标签/徽标的线：控件边界要满足 WCAG 1.4.11 的 3:1（极淡墨色刚好达标）。 */
  const controlLine = palette.inkFaint;

  return {
    common: {
      // 主色 = 墨色；hover/pressed 用次级墨，仍是中性石墨阶。
      primaryColor: palette.ink,
      primaryColorHover: palette.inkMuted,
      primaryColorPressed: palette.ink,
      primaryColorSuppl: palette.inkMuted,
      // 功能蓝只用于信息语义；hover/pressed 不派生蓝色填充（ADR-0012 约束 1）。
      infoColor: palette.link,
      infoColorHover: palette.inkMuted,
      infoColorPressed: palette.ink,
      infoColorSuppl: palette.link,
      // 五色锁死：success 走墨色，不引入绿 / 天蓝。
      successColor: palette.ink,
      successColorHover: palette.inkMuted,
      successColorPressed: palette.ink,
      successColorSuppl: palette.inkMuted,
      // 黄只做填充/底（三元组派生的不透明黄），hover/pressed 交给 Naive 从中性色派生。
      warningColor: warningFill,
      warningColorHover: warningFill,
      warningColorPressed: warningFill,
      warningColorSuppl: warningFill,
      // 危险 = 珊瑚橙；填充上的字在 Button 段统一换成墨色。
      errorColor: palette.danger,
      errorColorHover: palette.danger,
      errorColorPressed: palette.danger,
      errorColorSuppl: palette.danger,
      // 圆角阶梯（不再是 0）。
      borderRadius: scale.md,
      borderRadiusSmall: scale.sm,
      // 字体栈含中文 fallback。
      fontFamily: FONT_STACK,
      fontFamilyMono: FONT_MONO,
      // 中性面与墨色：全部来自语义层，明暗各一份取值。
      baseColor: palette.surface,
      bodyColor: palette.surface,
      cardColor: palette.surfaceRaised,
      modalColor: palette.surfaceRaised,
      popoverColor: palette.surfaceRaised,
      tableColor: palette.surfaceRaised,
      tableHeaderColor: palette.bgSubtle,
      inputColor: palette.surfaceRaised,
      inputColorDisabled: palette.bgSubtle,
      actionColor: palette.bgSubtle,
      tagColor: palette.bgSubtle,
      tabColor: palette.bgSubtle,
      textColor1: palette.ink,
      textColor2: palette.ink,
      textColor3: palette.inkMuted,
      placeholderColor: withTripletAlpha(triplets.graphite, ALPHA.placeholder),
      // 控件边界必须 3:1 以上（--color-line 只有 1.7:1，不够画输入框）。
      borderColor: controlLine,
      dividerColor: palette.line,
      tableColorHover: withTripletAlpha(triplets.graphite, ALPHA.tableHover),
      tableColorStriped: withTripletAlpha(triplets.graphite, ALPHA.tableStriped),
      hoverColor: withTripletAlpha(triplets.graphite, ALPHA.hover),
      pressedColor: withTripletAlpha(triplets.graphite, ALPHA.pressed),
      avatarColor: withTripletAlpha(triplets.graphite, ALPHA.avatar),
      codeColor: withTripletAlpha(triplets.graphite, ALPHA.code),
      buttonColor2: withTripletAlpha(triplets.graphite, ALPHA.button2),
      buttonColor2Hover: withTripletAlpha(triplets.graphite, ALPHA.button2Hover),
      buttonColor2Pressed: withTripletAlpha(triplets.graphite, ALPHA.button2Pressed),
      scrollbarColor: withTripletAlpha(triplets.graphite, ALPHA.scrollbar),
      scrollbarColorHover: withTripletAlpha(triplets.graphite, ALPHA.scrollbarHover),
      progressRailColor: withTripletAlpha(triplets.graphite, ALPHA.progressRail),
      railColor: withTripletAlpha(triplets.graphite, ALPHA.rail),
    },
    Button: {
      borderRadiusTiny: scale.sm,
      borderRadiusSmall: scale.sm,
      borderRadiusMedium: scale.md,
      borderRadiusLarge: scale.md,
      // filled 危险按钮：珊瑚橙底 + 墨色字（奶油白字只有 2.8:1）。
      textColorError: palette.onDanger,
      textColorHoverError: palette.onDanger,
      textColorFocusError: palette.onDanger,
      textColorPressedError: palette.onDanger,
      textColorDisabledError: palette.inkMuted,
      // tertiary / ghost / text 形态：暗橙字（暗色下是浅珊瑚），两态都达 AA。
      textColorTextError: palette.dangerInk,
      textColorTextHoverError: palette.dangerInk,
      textColorTextPressedError: palette.dangerInk,
      textColorTextFocusError: palette.dangerInk,
      textColorGhostError: palette.dangerInk,
      textColorGhostHoverError: palette.dangerInk,
      textColorGhostPressedError: palette.dangerInk,
      textColorGhostFocusError: palette.dangerInk,
      // 万一出现 warning 实心按钮：黄底必须配墨色字。
      textColorWarning: palette.ink,
      textColorHoverWarning: palette.ink,
      textColorFocusWarning: palette.ink,
      textColorPressedWarning: palette.ink,
    },
    Tag: {
      // 徽标是胶囊：走 pill 档。
      borderRadius: scale.pill,
      textColor: palette.ink,
      textColorPrimary: palette.ink,
      textColorSuccess: palette.ink,
      // 黄永不做字色：warning 标签文字用墨色。
      textColorWarning: palette.ink,
      // 珊瑚橙作字色不可达 AA，改用派生暗橙（暗色下是浅珊瑚）。
      textColorError: palette.dangerInk,
      colorWarning: withTripletAlpha(triplets.yellow, 0.15),
      colorError: palette.dangerSoft,
      colorSuccess: withTripletAlpha(triplets.graphite, 0.08),
      colorPrimary: withTripletAlpha(triplets.graphite, 0.08),
      borderWarning: `1px solid ${controlLine}`,
      borderError: `1px solid ${palette.dangerInk}`,
      borderSuccess: `1px solid ${controlLine}`,
      borderPrimary: `1px solid ${controlLine}`,
    },
    Alert: {
      borderRadius: scale.md,
      // 黄底警示：图标/标题/正文都用墨色（黄图标在浅黄底上读不出来）。
      iconColorWarning: palette.ink,
      titleTextColorWarning: palette.ink,
      contentTextColorWarning: palette.inkMuted,
      iconColorInfo: palette.ink,
      titleTextColorInfo: palette.ink,
      contentTextColorInfo: palette.inkMuted,
      iconColorSuccess: palette.ink,
      titleTextColorSuccess: palette.ink,
      contentTextColorSuccess: palette.inkMuted,
      iconColorError: palette.dangerInk,
      titleTextColorError: palette.ink,
      contentTextColorError: palette.inkMuted,
    },
    Card: { borderRadius: scale.lg },
    Input: { borderRadius: scale.md },
    Select: { peers: { InternalSelection: { borderRadius: scale.md } } },
    Dropdown: { borderRadius: scale.xl },
    Popover: { borderRadius: scale.xl },
    Modal: { borderRadius: scale.xl },
    Dialog: {
      borderRadius: scale.xl,
      // dialog.warning/error 的图标颜色默认直接取 warningColor/errorColor：
      // 黄图标在纸面上读不出来，珊瑚橙图标也不足 3:1，统一换墨色/暗橙。
      iconColor: palette.ink,
      iconColorInfo: palette.ink,
      iconColorSuccess: palette.ink,
      iconColorWarning: palette.ink,
      iconColorError: palette.dangerInk,
    },
    Message: {
      borderRadius: scale.md,
      iconColorWarning: palette.ink,
      iconColorError: palette.dangerInk,
      iconColorInfo: palette.ink,
      iconColorSuccess: palette.ink,
    },
  };
}
