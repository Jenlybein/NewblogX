import type { Component } from "vue";

import { MARKDOWN_THEME_OPTIONS } from "~/composables/useStudioMarkdownTheme";

import {
  IconAlignCenter,
  IconAlignLeft,
  IconAlignRight,
  IconBlockquote,
  IconBold,
  IconCodeDots,
  IconHeading,
  IconHighlight,
  IconItalic,
  IconLink,
  IconList,
  IconListNumbers,
  IconPalette,
  IconPhoto,
  IconSparkles,
  IconStrikethrough,
  IconTable,
  IconTopologyStar3,
} from "@tabler/icons-vue";

// 写作页的工具栏配置。
//
// 从 pages/studio/write.vue 抽出来（M6 ⑧）。这是**纯数据表**：
// 左边二十来个按钮、每个按钮的图标与下拉选项、以及快捷键对照。
// 放在页面里时，改一个按钮的文案要在 2000 行里翻，而且很容易
// 顺手把上面的 reactive 状态也带进 import 图里。
//
// 抽成独立模块后：
//   - 图标依赖只在这里，页面不再需要 import 20 个 Icon；
//   - 快捷键表与按钮表挨着，改快捷键时看得见两者的一致性；
//   - 这张表可以在测试里直接断言"每个 key 都有快捷键"或"没有重复 key"。

/** ToolOption 是下拉里的一个选项（例如标题级别、图表类型）。 */
export type ToolOption = {
  key: string;
  label: string;
  icon?: Component;
};

export type ToolItem = {
  key: string;
  label: string;
  icon: Component;
  options?: ToolOption[];
};

/** LEFT_TOOLS 是编辑区左侧的按钮，按展示顺序排列。 */
export const LEFT_TOOLS: ToolItem[] = [
  {
    key: "title",
    label: "标题",
    icon: IconHeading,
    options: [
      { key: "h1", label: "H1 一级标题" },
      { key: "h2", label: "H2 二级标题" },
      { key: "h3", label: "H3 三级标题" },
      { key: "h4", label: "H4 四级标题" },
      { key: "h5", label: "H5 五级标题" },
      { key: "h6", label: "H6 六级标题" },
    ],
  },
  { key: "bold", label: "加粗", icon: IconBold },
  { key: "italic", label: "斜体", icon: IconItalic },
  { key: "quote", label: "引用", icon: IconBlockquote },
  { key: "link", label: "链接", icon: IconLink },
  { key: "image", label: "图片", icon: IconPhoto },
  { key: "highlight", label: "高亮", icon: IconHighlight },
  { key: "code", label: "代码块", icon: IconCodeDots },
  { key: "ul", label: "无序列表", icon: IconList },
  { key: "ol", label: "有序列表", icon: IconListNumbers },
  { key: "strike", label: "删除线", icon: IconStrikethrough },
  { key: "table", label: "表格", icon: IconTable },
  {
    key: "ai",
    label: "AI 辅助",
    icon: IconSparkles,
    options: [
      { key: "polish", label: "润色改写" },
      { key: "grammar_fix", label: "语法纠错" },
      { key: "style_transform", label: "风格转换" },
      { key: "diagnose", label: "内容诊断" },
    ],
  },
  {
    key: "theme",
    label: "预览样式",
    icon: IconPalette,
    options: MARKDOWN_THEME_OPTIONS,
  },
  {
    key: "align",
    label: "对齐",
    icon: IconAlignLeft,
    options: [
      { key: "align-left", label: "左对齐", icon: IconAlignLeft },
      { key: "align-center", label: "居中对齐", icon: IconAlignCenter },
      { key: "align-right", label: "右对齐", icon: IconAlignRight },
    ],
  },
  {
    key: "mermaid",
    label: "Mermaid 图",
    icon: IconTopologyStar3,
    options: [
      { key: "flowchart", label: "流程图" },
      { key: "sequence", label: "时序图" },
      { key: "class", label: "类图" },
      { key: "state", label: "状态图" },
      { key: "er", label: "关系图" },
      { key: "journey", label: "旅程图" },
      { key: "gantt", label: "甘特图" },
      { key: "pie", label: "饼状图" },
    ],
  },
];

// 编辑器命令（选区替换 / Markdown 快捷键）见 composable。
//
// 抽出去的理由：这些函数只依赖 form.content 与 textarea 引用，
// 与文章元信息、封面、发布流程毫无关系。留在页面里时，
// "它到底会碰哪些状态"要靠通读 1200 行才能确认。

/** TOOL_SHORTCUTS 是按钮 key → 快捷键说明的对照表。 */
const TOOL_SHORTCUTS: Partial<Record<ToolItem["key"], string>> = {
  bold: "Ctrl+B",
  italic: "Ctrl+I",
  quote: "Ctrl+Q",
  link: "Ctrl+K",
  image: "Ctrl+Shift+I",
  highlight: "Ctrl+Shift+~",
  code: "Ctrl+Shift+K",
  ul: "Ctrl+L",
  ol: "Ctrl+Shift+L",
  strike: "Ctrl+D",
  table: "Ctrl+Alt+T",
};

/**
 * getToolTooltip 返回按钮的提示文案（带快捷键）。
 *
 * 没有配快捷键的按钮只显示名字 —— 不要在这里编一个默认值，
 * 提示里出现"假的快捷键"比没有更糟。
 */
export function getToolTooltip(tool: ToolItem) {
  const shortcut = TOOL_SHORTCUTS[tool.key];
  return shortcut ? `${tool.label} (${shortcut})` : tool.label;
}

