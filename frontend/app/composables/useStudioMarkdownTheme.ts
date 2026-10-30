import { computed, ref } from "vue";

import katexCssUrl from "katex/dist/katex.min.css?url";
import highlightCssUrl from "highlight.js/styles/github.min.css?url";
import githubMarkdownCssUrl from "github-markdown-css/github-markdown-light.css?url";
import githubMarkdownDarkCssUrl from "github-markdown-css/github-markdown-dark.css?url";
import githubMarkdownLightColorblindCssUrl from "github-markdown-css/github-markdown-light-colorblind.css?url";
import githubMarkdownDarkColorblindCssUrl from "github-markdown-css/github-markdown-dark-colorblind.css?url";
import githubMarkdownDarkDimmedCssUrl from "github-markdown-css/github-markdown-dark-dimmed.css?url";
import githubMarkdownDarkHighContrastCssUrl from "github-markdown-css/github-markdown-dark-high-contrast.css?url";
import markdownThemeShanyueCssUrl from "markdown-theme/themes/shanyue.css?url";
import markdownThemeVGreenCssUrl from "markdown-theme/themes/v-green.css?url";
import markdownThemeChocolateCssUrl from "markdown-theme/themes/chocolate.css?url";
import markdownThemeShanchuiCssUrl from "markdown-theme/themes/shanchui.css?url";
import markdownThemeMenglvCssUrl from "markdown-theme/themes/menglv.css?url";
import markdownThemeCondensedNightPurpleCssUrl from "markdown-theme/themes/condensed-night-purple.css?url";

// Markdown 渲染主题的选项与 CSS 映射。
//
// 从 pages/studio/write.vue 抽出来（M6 ⑧）。它是一张**纯数据表**：
// 12 个主题各自对应哪个 CSS 文件、显示什么名字。放在页面里时，
// 每次改主题都要在 2000 行里找到那张表，还得确认没碰坏别的东西。
//
// 这类表还有个隐性风险：主题 key 与 CSS 必须一一对应。
// 抽成独立模块后，可以单独加一条"每个 key 都有 href"的测试兜住。

// 主题的 key。与 CSS 文件名一一对应，改名要同步改 mapping。
// `jelly` 是 #8 的自有 markdown 主题（token 驱动、无外部 CSS 地址），作为默认值；
// 其余 12 套通用主题保留给需要"别的样式"的场景（显式选择时才加载外部 <link>）。
export type MarkdownThemeKey =
  | "jelly"
  | "github"
  | "github-dark"
  | "github-colorblind"
  | "github-dark-colorblind"
  | "github-dark-dimmed"
  | "github-dark-high-contrast"
  | "shanyue"
  | "vgreen"
  | "chocolate"
  | "shanchui"
  | "menglv"
  | "condensed-night-purple";

export type ThemeOption = {
  key: MarkdownThemeKey;
  label: string;
};

// 主题的 CSS 资源。
//
// 用 `?url` 让 Vite 输出成"可切换的 <link href>"，
// 而不是打进主样式表 —— 12 套主题全打进首屏是几百 KB 的浪费。

/**
 * MARKDOWN_THEME_OPTIONS 是可选主题的展示名。
 *
 * 提成模块级常量而不是留在 composable 里：工具栏（constants/studioToolbar）的
 * "预览样式"下拉要引用同一张表。如果它只存在于 composable 内部，
 * 工具栏就得自己抄一份 —— 两份表迟早会不一致（表现为"下拉里有个主题，
 * 但选中后没有 CSS"）。
 */
export const MARKDOWN_THEME_OPTIONS: ThemeOption[] = [
  { key: "jelly", label: "JellyBin 图纸（默认）" },
  { key: "github", label: "GitHub Light" },
  { key: "github-dark", label: "GitHub Dark" },
  { key: "github-colorblind", label: "GitHub Colorblind" },
  { key: "github-dark-colorblind", label: "GitHub Dark Colorblind" },
  { key: "github-dark-dimmed", label: "GitHub Dark Dimmed" },
  { key: "github-dark-high-contrast", label: "GitHub Dark High Contrast" },
  { key: "shanyue", label: "Shanyue" },
  { key: "vgreen", label: "V-Green" },
  { key: "chocolate", label: "Chocolate" },
  { key: "shanchui", label: "Shanchui" },
  { key: "menglv", label: "Menglv" },
  { key: "condensed-night-purple", label: "Condensed Night Purple" },
];

export function useStudioMarkdownTheme() {
  const selectedMarkdownTheme = ref<MarkdownThemeKey>("jelly");

  const markdownThemeOptions = MARKDOWN_THEME_OPTIONS;

  const visibilityOptions = [
    { label: "公开展示", value: "visible" },
    { label: "仅自己可见", value: "user_hidden" },
  ];

  const selectedThemeLabel = computed(
    () => markdownThemeOptions.find((item) => item.key === selectedMarkdownTheme.value)?.label || "JellyBin 图纸（默认）",
  );

  const markdownThemeHrefMap: Record<typeof selectedMarkdownTheme.value, string> = {
    // 自有主题：不加载外部 CSS，配色完全由 shadow 内的 token 主题决定。
    jelly: "",
    github: githubMarkdownCssUrl,
    "github-dark": githubMarkdownDarkCssUrl,
    "github-colorblind": githubMarkdownLightColorblindCssUrl,
    "github-dark-colorblind": githubMarkdownDarkColorblindCssUrl,
    "github-dark-dimmed": githubMarkdownDarkDimmedCssUrl,
    "github-dark-high-contrast": githubMarkdownDarkHighContrastCssUrl,
    shanyue: markdownThemeShanyueCssUrl,
    vgreen: markdownThemeVGreenCssUrl,
    chocolate: markdownThemeChocolateCssUrl,
    shanchui: markdownThemeShanchuiCssUrl,
    menglv: markdownThemeMenglvCssUrl,
    "condensed-night-purple": markdownThemeCondensedNightPurpleCssUrl,
  };

  const activeMarkdownThemeHref = computed(() => markdownThemeHrefMap[selectedMarkdownTheme.value]);
  // 自有主题自带代码高亮配色；只有显式选择 GitHub 等通用主题时才需要外部的 highlight.css。
  const markdownSupportStyleHrefs = computed(() =>
    selectedMarkdownTheme.value === "jelly" ? [katexCssUrl] : [katexCssUrl, highlightCssUrl],
  );

  return {
    selectedMarkdownTheme,
    markdownThemeOptions,
    visibilityOptions,
    selectedThemeLabel,
    markdownThemeHrefMap,
    activeMarkdownThemeHref,
    markdownSupportStyleHrefs,
  };
}
