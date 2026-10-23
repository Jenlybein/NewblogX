<script setup lang="ts">
import type { Component } from "vue";
import type { ArticleHeadingAnchor } from "~/composables/useArticleMarkdown";
import type { StudioAiAction } from "~/composables/useStudioAiSelection";
import { computed, defineAsyncComponent, nextTick, reactive, ref, watch } from "vue";
import { useDebounce } from "@vueuse/core";
import {
  IconAlignCenter,
  IconAlignLeft,
  IconAlignRight,
  IconBlockquote,
  IconBold,
  IconCodeDots,
  IconColumns1,
  IconColumns2,
  IconHeading,
  IconHighlight,
  IconItalic,
  IconLink,
  IconList,
  IconListDetails,
  IconListNumbers,
  IconPalette,
  IconPhoto,
  IconSparkles,
  IconStrikethrough,
  IconTable,
  IconTopologyStar3,
} from "@tabler/icons-vue";
import { NButton, NCard, NInput, NModal, NPopover, NSelect, NSwitch, NTooltip, useMessage } from "naive-ui";
import AppAvatar from "~/components/common/AppAvatar.vue";
import ArticleScoreModal from "~/components/article/ArticleScoreModal.vue";
import StudioAiAssistPanel from "~/components/studio/StudioAiAssistPanel.vue";
import StudioSelectionAiToolbar from "~/components/studio/StudioSelectionAiToolbar.vue";
import { useStudioAiSelection } from "~/composables/useStudioAiSelection";
import { useTextareaSelectionOverlay } from "~/composables/useTextareaSelectionOverlay";
import { generateArticleMetainfo, getArticleScoreDetail, regenerateArticleScore } from "~/services/ai";
import { createArticle, getArticleDetail, updateArticle } from "~/services/article";
import { uploadImageByTask } from "~/services/image";
import { getCategoryOptions, getTagOptions } from "~/services/search";
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
import { resolveAvatarInitial } from "~/utils/avatar";

definePageMeta({
  layout: "write",
  middleware: "auth",
});

type ToolOption = {
  key: string;
  label: string;
  icon?: Component;
};

type ToolItem = {
  key: string;
  label: string;
  icon: Component;
  options?: ToolOption[];
};

const router = useRouter();
const route = useRoute();
const WriteMarkdownRenderer = defineAsyncComponent(() => import("~/components/common/MarkdownRenderSurface.vue"));
const authStore = useAuthStore();
const message = useMessage();
const editorTextareaRef = ref<HTMLTextAreaElement | null>(null);
const coverFileInputRef = ref<HTMLInputElement | null>(null);
const shadowPreviewRef = ref<{ scrollToHeading: (id: string) => boolean } | null>(null);
const previewHeadings = ref<ArticleHeadingAnchor[]>([]);

if (!authStore.profileId) {
  await authStore.fetchCurrentUser();
}

const form = reactive({
  title: "",
  abstract: "",
  content: "",
  category_id: null as string | null,
  tag_ids: [] as string[],
  cover_image_id: null as string | null,
  comments_toggle: true,
  visibility_status: "visible" as "visible" | "user_hidden",
});

const pendingState = reactive({
  draft: false,
  publish: false,
  ai: false,
  cover: false,
});

const coverPreviewUrl = ref("");
const coverUploadStage = ref("");
const articleScoreModalOpen = ref(false);
const articleScoreLoading = ref(false);
const articleScoreRefreshing = ref(false);
const articleScoreData = ref<Awaited<ReturnType<typeof getArticleScoreDetail>> | null>(null);

const showToc = ref(true);
const viewMode = ref<"split" | "editor" | "preview">("split");
const publishVisible = ref(false);
const coverDirty = ref(false);
const selectedMarkdownTheme = ref<
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
  | "condensed-night-purple"
>("github");

const currentUserId = computed(() => authStore.profileId || "");
const currentUserInitial = computed(() => resolveAvatarInitial(authStore.profileName, "我"));
const editArticleId = computed(() => {
  const fromArticleId = typeof route.query.article_id === "string" ? route.query.article_id.trim() : "";
  if (fromArticleId) return fromArticleId;
  const fromId = typeof route.query.id === "string" ? route.query.id.trim() : "";
  return fromId || "";
});
const isEditMode = computed(() => Boolean(editArticleId.value));
const hydratedArticleId = ref("");

const { data: tagOptions } = await useAsyncData("studio-write-tag-options", () => getTagOptions().catch(() => []));
const { data: categoryOptions } = await useAsyncData(
  () => `studio-write-category-options:${currentUserId.value || "guest"}`,
  () => (currentUserId.value ? getCategoryOptions(currentUserId.value).catch(() => []) : Promise.resolve([])),
  {
    watch: [currentUserId],
  },
);

const { data: editingArticle, error: editingArticleError } = await useAsyncData(
  () => `studio-write-article-detail:${editArticleId.value || "new"}`,
  () => (editArticleId.value ? getArticleDetail(editArticleId.value) : Promise.resolve(null)),
  {
    watch: [editArticleId],
  },
);

watch(
  editingArticleError,
  (error) => {
    if (!error || !isEditMode.value) return;
    message.error(error instanceof Error ? error.message : "加载待编辑文章失败");
  },
  { immediate: true },
);

watch(
  [editArticleId, editingArticle],
  ([articleId, article]) => {
    if (!articleId) {
      hydratedArticleId.value = "";
      coverPreviewUrl.value = "";
      form.cover_image_id = null;
      form.visibility_status = "visible";
      coverDirty.value = false;
      return;
    }
    if (!article || hydratedArticleId.value === articleId) {
      return;
    }

    form.title = article.title || "";
    form.abstract = article.abstract || "";
    form.content = article.content || "";
    form.cover_image_id = article.cover_image_id ?? null;
    coverPreviewUrl.value = article.cover || "";
    coverDirty.value = false;
    form.comments_toggle = article.comments_toggle ?? true;
    form.visibility_status = article.visibility_status === "user_hidden" ? "user_hidden" : "visible";
    form.category_id = article.category_id ?? null;
    form.tag_ids = Array.isArray(article.tag_ids) ? [...article.tag_ids] : [];
    hydratedArticleId.value = articleId;
  },
  { immediate: true },
);

watch(editArticleId, () => {
  articleScoreData.value = null;
  articleScoreModalOpen.value = false;
});

const debouncedContent = useDebounce(computed(() => form.content), 300);

const writeBodyClass = computed(() => ({
  "write-page__body--hide-toc": !showToc.value,
  "write-page__body--editor-only": viewMode.value === "editor",
  "write-page__body--preview-only": viewMode.value === "preview",
}));

const contentStats = computed(() => ({
  raw: form.content.length,
  pure: form.content.replace(/\s/g, "").length,
  lines: form.content ? form.content.split("\n").length : 1,
}));

const tocItems = computed(() => {
  return previewHeadings.value.map((item) => ({
    id: item.id,
    label: item.title,
    level: item.level,
  }));
});

const canSubmit = computed(() => Boolean(form.title.trim() && form.content.trim()));
const draftActionLabel = computed(() => (isEditMode.value ? "更新草稿" : "保存草稿"));
const publishActionLabel = computed(() => (isEditMode.value ? "更新并发布" : "确定并发布"));
const isPublishingLocked = computed(() => pendingState.publish || pendingState.draft || pendingState.ai || pendingState.cover);
const canOpenArticleScore = computed(() => Boolean(editArticleId.value));
const currentArticlePublishStatus = computed(() => editingArticle.value?.publish_status ?? 0);

const markdownThemeOptions: ToolOption[] = [
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

const visibilityOptions = [
  { label: "公开展示", value: "visible" },
  { label: "仅自己可见", value: "user_hidden" },
];

const selectedThemeLabel = computed(
  () => markdownThemeOptions.find((item) => item.key === selectedMarkdownTheme.value)?.label || "GitHub Light",
);

const markdownThemeHrefMap: Record<typeof selectedMarkdownTheme.value, string> = {
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
const markdownSupportStyleHrefs = [katexCssUrl, highlightCssUrl];

const leftTools: ToolItem[] = [
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
    options: markdownThemeOptions,
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

function focusEditor(selectionStart?: number, selectionEnd?: number) {
  nextTick(() => {
    const textarea = editorTextareaRef.value;
    if (!textarea) {
      return;
    }
    textarea.focus();
    if (typeof selectionStart === "number" && typeof selectionEnd === "number") {
      textarea.setSelectionRange(selectionStart, selectionEnd);
    }
  });
}

const articleTitleModel = computed({
  get: () => form.title,
  set: (value: string) => {
    form.title = value;
  },
});

const articleContentModel = computed({
  get: () => form.content,
  set: (value: string) => {
    form.content = value;
  },
});

const {
  panelVisible: aiPanelVisible,
  currentAction: aiPanelAction,
  styleInstruction: aiStyleInstruction,
  selectionContext: aiSelectionContext,
  overwriteResult: aiOverwriteResult,
  diagnoseResult: aiDiagnoseResult,
  errorMessage: aiErrorMessage,
  isBusy: aiPanelBusy,
  canSubmit: aiCanSubmit,
  openPanel: openAiPanel,
  closePanel: closeAiPanel,
  runCurrentAction: runAiPanelAction,
  applyOverwriteResult: applyAiOverwriteResult,
  insertOverwriteBelow: insertAiOverwriteBelow,
  copyOverwriteResult: copyAiOverwriteResult,
} = useStudioAiSelection({
  content: articleContentModel,
  title: articleTitleModel,
  textareaRef: editorTextareaRef,
  focusEditor,
});

const {
  selectionOverlay,
  updateSelectionOverlay,
  hideSelectionOverlay,
} = useTextareaSelectionOverlay(editorTextareaRef, {
  hideWhen: () => aiPanelVisible.value || publishVisible.value || viewMode.value === "preview",
  ignoreSelectors: [".studio-selection-ai-toolbar", ".studio-ai-modal"],
});

watch([aiPanelVisible, publishVisible, viewMode], ([isAiOpen, isPublishOpen, mode]) => {
  if (isAiOpen || isPublishOpen || mode === "preview") {
    hideSelectionOverlay();
    return;
  }

  nextTick(() => {
    updateSelectionOverlay();
  });
});

watch(
  () => form.content,
  () => {
    nextTick(() => {
      updateSelectionOverlay();
    });
  },
);

function replaceSelection(transform: (selectedText: string) => { content: string; start: number; end: number }) {
  const textarea = editorTextareaRef.value;
  const fallbackSelection = form.content.length;
  const selectionStart = textarea?.selectionStart ?? fallbackSelection;
  const selectionEnd = textarea?.selectionEnd ?? fallbackSelection;
  const selectedText = form.content.slice(selectionStart, selectionEnd);
  const nextSelection = transform(selectedText);

  form.content = `${form.content.slice(0, selectionStart)}${nextSelection.content}${form.content.slice(selectionEnd)}`;
  focusEditor(selectionStart + nextSelection.start, selectionStart + nextSelection.end);
}

function insertSnippet(snippet: string) {
  replaceSelection((selectedText) => {
    const content = selectedText ? `${snippet}${selectedText}` : snippet;
    const offset = content.length;
    return { content, start: offset, end: offset };
  });
}

function wrapSelection(prefix: string, suffix: string, placeholder: string) {
  replaceSelection((selectedText) => {
    const value = selectedText || placeholder;
    return {
      content: `${prefix}${value}${suffix}`,
      start: prefix.length,
      end: prefix.length + value.length,
    };
  });
}

function findWordRangeAtCursor(content: string, cursor: number) {
  const isWordChar = (char: string) => /[A-Za-z0-9_\u4e00-\u9fff]/.test(char);
  let start = cursor;
  let end = cursor;

  while (start > 0 && isWordChar(content[start - 1] || "")) {
    start -= 1;
  }

  while (end < content.length && isWordChar(content[end] || "")) {
    end += 1;
  }

  if (start === end) {
    return null;
  }

  return { start, end };
}

function getShortcutRange() {
  const textarea = editorTextareaRef.value;
  if (!textarea) {
    return null;
  }

  let selectionStart = textarea.selectionStart ?? 0;
  let selectionEnd = textarea.selectionEnd ?? selectionStart;

  if (selectionStart === selectionEnd) {
    const wordRange = findWordRangeAtCursor(form.content, selectionStart);
    if (!wordRange) {
      return null;
    }
    selectionStart = wordRange.start;
    selectionEnd = wordRange.end;
  }

  return { selectionStart, selectionEnd };
}

function toggleInlineWrapper(prefix: string, suffix: string) {
  const range = getShortcutRange();
  if (!range) {
    return;
  }

  const { selectionStart, selectionEnd } = range;
  const content = form.content;
  const hasWrappedSelection =
    selectionStart >= prefix.length &&
    selectionEnd + suffix.length <= content.length &&
    content.slice(selectionStart - prefix.length, selectionStart) === prefix &&
    content.slice(selectionEnd, selectionEnd + suffix.length) === suffix;

  if (hasWrappedSelection) {
    const nextContent =
      content.slice(0, selectionStart - prefix.length) +
      content.slice(selectionStart, selectionEnd) +
      content.slice(selectionEnd + suffix.length);
    form.content = nextContent;
    focusEditor(selectionStart - prefix.length, selectionEnd - prefix.length);
    return;
  }

  const selectedText = content.slice(selectionStart, selectionEnd);
  form.content = `${content.slice(0, selectionStart)}${prefix}${selectedText}${suffix}${content.slice(selectionEnd)}`;
  focusEditor(selectionStart + prefix.length, selectionEnd + prefix.length);
}

function applyBoldShortcut() {
  toggleInlineWrapper("**", "**");
}

function applyItalicShortcut() {
  toggleInlineWrapper("*", "*");
}

function applyUnderlineShortcut() {
  toggleInlineWrapper("++", "++");
}

function applyInlineCodeShortcut() {
  toggleInlineWrapper("`", "`");
}

function applyImageShortcut() {
  insertSnippet("![图片描述](https://example.com/image.png)\n");
}

function applyStrikeShortcut() {
  toggleInlineWrapper("~~", "~~");
}

function applyTableShortcut() {
  insertSnippet("| 列1 | 列2 |\n| --- | --- |\n| 内容 | 内容 |\n");
}

function applyHeadingShortcut(level: number) {
  const textarea = editorTextareaRef.value;
  if (!textarea) {
    return;
  }

  const selectionStart = textarea.selectionStart ?? 0;
  const selectionEnd = textarea.selectionEnd ?? selectionStart;
  const content = form.content;
  const lineStart = content.lastIndexOf("\n", Math.max(0, selectionStart - 1)) + 1;
  let lineEnd = content.indexOf("\n", selectionEnd);
  if (lineEnd === -1) {
    lineEnd = content.length;
  }

  const lineBlock = content.slice(lineStart, lineEnd);
  const nextPrefix = `${"#".repeat(level)} `;
  const transformedBlock = lineBlock
    .split("\n")
    .map((line) => {
      if (!line.trim()) {
        return line;
      }
      const stripped = line.replace(/^\s{0,3}#{1,6}\s+/, "");
      return `${nextPrefix}${stripped}`;
    })
    .join("\n");

  form.content = `${content.slice(0, lineStart)}${transformedBlock}${content.slice(lineEnd)}`;
  focusEditor(lineStart, lineStart + transformedBlock.length);
}

function replaceCurrentLineBlock(transformLine: (line: string, index: number) => string) {
  const textarea = editorTextareaRef.value;
  if (!textarea) {
    return;
  }

  const selectionStart = textarea.selectionStart ?? 0;
  const selectionEnd = textarea.selectionEnd ?? selectionStart;
  const content = form.content;
  const lineStart = content.lastIndexOf("\n", Math.max(0, selectionStart - 1)) + 1;
  let lineEnd = content.indexOf("\n", selectionEnd);
  if (lineEnd === -1) {
    lineEnd = content.length;
  }

  const lineBlock = content.slice(lineStart, lineEnd);
  const transformedBlock = lineBlock
    .split("\n")
    .map((line, index) => transformLine(line, index))
    .join("\n");

  form.content = `${content.slice(0, lineStart)}${transformedBlock}${content.slice(lineEnd)}`;
  focusEditor(lineStart, lineStart + transformedBlock.length);
}

function applyQuoteShortcut() {
  replaceCurrentLineBlock((line) => {
    if (!line.trim()) {
      return line;
    }
    if (/^\s{0,3}>\s?/.test(line)) {
      return line.replace(/^\s{0,3}>\s?/, "");
    }
    return `> ${line}`;
  });
}

function applyUnorderedListShortcut() {
  const textarea = editorTextareaRef.value;
  if (!textarea) {
    return;
  }

  const selectionStart = textarea.selectionStart ?? 0;
  const selectionEnd = textarea.selectionEnd ?? selectionStart;
  const content = form.content;
  const lineStart = content.lastIndexOf("\n", Math.max(0, selectionStart - 1)) + 1;
  let lineEnd = content.indexOf("\n", selectionEnd);
  if (lineEnd === -1) {
    lineEnd = content.length;
  }
  const lineBlock = content.slice(lineStart, lineEnd);
  const lines = lineBlock.split("\n");
  const nonEmptyLines = lines.filter((line) => line.trim());
  const isAllBulleted = nonEmptyLines.length > 0 && nonEmptyLines.every((line) => /^\s*-\s+/.test(line));

  replaceCurrentLineBlock((line) => {
    if (!line.trim()) {
      return line;
    }
    if (isAllBulleted) {
      return line.replace(/^\s*-\s+/, "");
    }
    return `- ${line}`;
  });
}

function applyOrderedListShortcut() {
  const textarea = editorTextareaRef.value;
  if (!textarea) {
    return;
  }

  const selectionStart = textarea.selectionStart ?? 0;
  const selectionEnd = textarea.selectionEnd ?? selectionStart;
  const content = form.content;
  const lineStart = content.lastIndexOf("\n", Math.max(0, selectionStart - 1)) + 1;
  let lineEnd = content.indexOf("\n", selectionEnd);
  if (lineEnd === -1) {
    lineEnd = content.length;
  }
  const lineBlock = content.slice(lineStart, lineEnd);
  const lines = lineBlock.split("\n");
  const nonEmptyLines = lines.filter((line) => line.trim());
  const isAllOrdered = nonEmptyLines.length > 0 && nonEmptyLines.every((line) => /^\s*\d+\.\s+/.test(line));
  let order = 0;

  replaceCurrentLineBlock((line) => {
    if (!line.trim()) {
      return line;
    }
    if (isAllOrdered) {
      return line.replace(/^\s*\d+\.\s+/, "");
    }
    order += 1;
    return `${order}. ${line}`;
  });
}

function applyCodeBlockShortcut() {
  const textarea = editorTextareaRef.value;
  const fallbackSelection = form.content.length;
  const selectionStart = textarea?.selectionStart ?? fallbackSelection;
  const selectionEnd = textarea?.selectionEnd ?? fallbackSelection;
  const selectedText = form.content.slice(selectionStart, selectionEnd);
  const match = selectedText.match(/^```([\w-]*)\n([\s\S]*?)\n```$/);

  if (match) {
    const inner = match[2] || "";
    form.content = `${form.content.slice(0, selectionStart)}${inner}${form.content.slice(selectionEnd)}`;
    focusEditor(selectionStart, selectionStart + inner.length);
    return;
  }

  const code = selectedText || "const answer = true;";
  const wrapped = `\`\`\`ts\n${code}\n\`\`\``;
  form.content = `${form.content.slice(0, selectionStart)}${wrapped}${form.content.slice(selectionEnd)}`;
  focusEditor(selectionStart + 6, selectionStart + 6 + code.length);
}

function applyLinkShortcut() {
  const textarea = editorTextareaRef.value;
  if (!textarea) {
    return;
  }

  const fallbackSelection = form.content.length;
  let selectionStart = textarea.selectionStart ?? fallbackSelection;
  let selectionEnd = textarea.selectionEnd ?? fallbackSelection;

  if (selectionStart === selectionEnd) {
    const wordRange = findWordRangeAtCursor(form.content, selectionStart);
    if (wordRange) {
      selectionStart = wordRange.start;
      selectionEnd = wordRange.end;
    }
  }

  const selectedText = form.content.slice(selectionStart, selectionEnd);
  const linkMatch = selectedText.match(/^\[([^\]]+)\]\(([^)]+)\)$/);
  const defaultText = linkMatch?.[1] || selectedText || "链接文本";
  const defaultUrl = linkMatch?.[2] || "https://example.com";
  const url = import.meta.client ? window.prompt("请输入链接 URL", defaultUrl) : defaultUrl;

  if (!url) {
    return;
  }

  const nextContent = `[${defaultText}](${url.trim()})`;
  form.content = `${form.content.slice(0, selectionStart)}${nextContent}${form.content.slice(selectionEnd)}`;
  focusEditor(selectionStart + 1, selectionStart + 1 + defaultText.length);
}

function handleEditorShortcuts(event: KeyboardEvent) {
  if (!(event.ctrlKey || event.metaKey)) {
    return;
  }

  if (event.altKey) {
    if (event.key.toLowerCase() === "t") {
      event.preventDefault();
      applyTableShortcut();
    }
    return;
  }

  if (event.key.toLowerCase() === "b") {
    event.preventDefault();
    applyBoldShortcut();
    return;
  }

  if (event.shiftKey && event.key.toLowerCase() === "i") {
    event.preventDefault();
    applyImageShortcut();
    return;
  }

  if (event.key.toLowerCase() === "i") {
    event.preventDefault();
    applyItalicShortcut();
    return;
  }

  if (event.key.toLowerCase() === "u") {
    event.preventDefault();
    applyUnderlineShortcut();
    return;
  }

  if (event.shiftKey && event.key.toLowerCase() === "k") {
    event.preventDefault();
    applyCodeBlockShortcut();
    return;
  }

  if (event.shiftKey && event.key.toLowerCase() === "l") {
    event.preventDefault();
    applyOrderedListShortcut();
    return;
  }

  if (event.shiftKey && (event.key === "~" || event.code === "Backquote")) {
    event.preventDefault();
    applyInlineCodeShortcut();
    return;
  }

  if (event.key.toLowerCase() === "q") {
    event.preventDefault();
    applyQuoteShortcut();
    return;
  }

  if (event.key.toLowerCase() === "k") {
    event.preventDefault();
    applyLinkShortcut();
    return;
  }

  if (event.key.toLowerCase() === "l") {
    event.preventDefault();
    applyUnorderedListShortcut();
    return;
  }

  if (event.key.toLowerCase() === "d") {
    event.preventDefault();
    applyStrikeShortcut();
    return;
  }

  if (/^[1-6]$/.test(event.key)) {
    event.preventDefault();
    applyHeadingShortcut(Number(event.key));
  }
}

const toolShortcutMap: Partial<Record<ToolItem["key"], string>> = {
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

function getToolTooltip(tool: ToolItem) {
  const shortcut = toolShortcutMap[tool.key];
  return shortcut ? `${tool.label} (${shortcut})` : tool.label;
}

async function handleAiToolOpen(action: StudioAiAction) {
  try {
    hideSelectionOverlay();
    await openAiPanel(action);
  } catch (error) {
    message.warning(error instanceof Error ? error.message : "AI 辅助暂时不可用");
  }
}

function handleSelectionToolbarAction(action: StudioAiAction) {
  void handleAiToolOpen(action);
}

function handleEditorSelectionChange() {
  updateSelectionOverlay();
}

async function handleAiPanelRun() {
  try {
    await runAiPanelAction();
  } catch (error) {
    if (error instanceof Error && error.message) {
      message.error(error.message);
      return;
    }
    message.error("AI 辅助执行失败");
  }
}

function handleAiReplace() {
  try {
    applyAiOverwriteResult();
    message.success("改写结果已替换到正文中");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "替换选区失败");
  }
}

function handleAiInsertBelow() {
  try {
    insertAiOverwriteBelow();
    message.success("改写结果已插入到选区下方");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "插入正文失败");
  }
}

async function handleAiCopyResult() {
  try {
    await copyAiOverwriteResult();
    message.success("改写结果已复制");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "复制失败");
  }
}

function handleAiPanelVisibility(nextVisible: boolean) {
  if (nextVisible) {
    aiPanelVisible.value = true;
    hideSelectionOverlay();
    return;
  }

  closeAiPanel();
  nextTick(() => {
    updateSelectionOverlay();
  });
}

function handleToolOption(toolKey: string, optionKey: string) {
  if (toolKey === "ai") {
    void handleAiToolOpen(optionKey as StudioAiAction);
    return;
  }

  if (toolKey === "theme") {
    selectedMarkdownTheme.value =
      (markdownThemeOptions.find((option) => option.key === optionKey)?.key as typeof selectedMarkdownTheme.value) ||
      "github";
    return;
  }

  if (toolKey === "title") {
    const level = Number(optionKey.slice(1)) || 2;
    insertSnippet(`${"#".repeat(level)} 标题\n`);
    return;
  }

  if (toolKey === "align") {
    const alignMap = {
      "align-left": "<div align=\"left\">\n内容\n</div>\n",
      "align-center": "<div align=\"center\">\n内容\n</div>\n",
      "align-right": "<div align=\"right\">\n内容\n</div>\n",
    } as const;

    insertSnippet(alignMap[optionKey as keyof typeof alignMap] || "<div align=\"left\">\n内容\n</div>\n");
    return;
  }

  if (toolKey === "mermaid") {
    const mermaidMap = {
      flowchart: "flowchart TD\n  A[开始] --> B[继续创作]",
      sequence: "sequenceDiagram\n  participant U as User\n  participant A as API\n  U->>A: 请求数据\n  A-->>U: 返回结果",
      class: "classDiagram\n  class Article {\n    +string title\n    +string content\n  }",
      state: "stateDiagram-v2\n  [*] --> Draft\n  Draft --> Published",
      er: "erDiagram\n  USER ||--o{ ARTICLE : writes",
      journey: "journey\n  title 创作流程\n  section 写作\n    起草: 5: 作者\n    发布: 4: 作者",
      gantt: "gantt\n  title 发布计划\n  dateFormat  YYYY-MM-DD\n  section Draft\n  写初稿 :2026-04-14, 2d",
      pie: "pie title 标签占比\n  \"Nuxt\" : 40\n  \"OpenAPI\" : 30\n  \"TypeScript\" : 30",
    } as const;

    insertSnippet(`\`\`\`mermaid\n${mermaidMap[optionKey as keyof typeof mermaidMap] || mermaidMap.flowchart}\n\`\`\`\n`);
  }
}

function isToolOptionActive(toolKey: string, optionKey: string) {
  if (toolKey === "theme") {
    return selectedMarkdownTheme.value === optionKey;
  }
  return false;
}

function handleToolClick(tool: ToolItem) {
  switch (tool.key) {
    case "bold":
      applyBoldShortcut();
      break;
    case "italic":
      applyItalicShortcut();
      break;
    case "quote":
      applyQuoteShortcut();
      break;
    case "link":
      applyLinkShortcut();
      break;
    case "image":
      applyImageShortcut();
      break;
    case "highlight":
      applyInlineCodeShortcut();
      break;
    case "code":
      applyCodeBlockShortcut();
      break;
    case "ul":
      applyUnorderedListShortcut();
      break;
    case "ol":
      applyOrderedListShortcut();
      break;
    case "strike":
      applyStrikeShortcut();
      break;
    case "table":
      applyTableShortcut();
      break;
    default:
      break;
  }
}

function handleRightToolClick(key: "toc" | "editor" | "preview") {
  if (key === "toc") {
    showToc.value = !showToc.value;
    return;
  }

  const nextMode = key === "editor" ? "editor" : "preview";
  viewMode.value = viewMode.value === nextMode ? "split" : nextMode;
}

function handleTocJump(id: string) {
  if (!id) {
    focusEditor();
    return;
  }

  shadowPreviewRef.value?.scrollToHeading(id);
}

function handlePreviewHeadingsChange(nextHeadings: ArticleHeadingAnchor[]) {
  previewHeadings.value = nextHeadings;
}

function matchOptionByText(options: Array<{ label: string; value: string }>, target?: { id?: string; title?: string } | null) {
  if (!target) return null;
  const byId = options.find((item) => item.value === target.id);
  if (byId) return byId.value;
  const normalizedTitle = target.title?.trim().toLowerCase();
  const byTitle = options.find((item) => item.label.trim().toLowerCase() === normalizedTitle);
  return byTitle?.value ?? null;
}

async function handleAiAssist() {
  if (!form.content.trim()) {
    message.warning("先写一点正文，再让 AI 帮你补标题和摘要。");
    return;
  }

  pendingState.ai = true;
  try {
    const result = await generateArticleMetainfo(form.content);
    form.title = result.title?.trim() || form.title;
    form.abstract = result.abstract?.trim() || form.abstract;

    const matchedCategoryId = matchOptionByText(categoryOptions.value || [], result.category);
    if (matchedCategoryId) {
      form.category_id = matchedCategoryId;
    }

    const nextTagIds = (result.tags || [])
      .map((item) => matchOptionByText(tagOptions.value || [], item))
      .filter((item): item is string => Boolean(item));

    if (nextTagIds.length) {
      form.tag_ids = Array.from(new Set(nextTagIds));
    }

    message.success("AI 已根据正文补全元信息建议");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "AI 元信息生成失败");
  } finally {
    pendingState.ai = false;
  }
}

async function fetchStoredArticleScore() {
  if (!editArticleId.value) {
    throw new Error("请先保存当前文章，再查看历史评分。");
  }

  articleScoreData.value = await getArticleScoreDetail(editArticleId.value);
}

async function handleOpenArticleScore() {
  if (!editArticleId.value) {
    message.info("新建文章暂时没有 article_id，请先保存为草稿或更新文章后再查看评分。");
    return;
  }

  articleScoreModalOpen.value = true;
  articleScoreLoading.value = true;
  try {
    await fetchStoredArticleScore();
  } catch (error) {
    message.error(error instanceof Error ? error.message : "读取评分失败");
  } finally {
    articleScoreLoading.value = false;
  }
}

async function handleRefreshArticleScore() {
  if (!editArticleId.value) {
    message.info("请先保存文章后再重新获取评分。");
    return;
  }

  articleScoreRefreshing.value = true;
  try {
    if (currentArticlePublishStatus.value !== 1) {
      await updateArticle(editArticleId.value, buildArticlePayload(1));
      if (editingArticle.value) {
        editingArticle.value = {
          ...editingArticle.value,
          title: form.title.trim(),
          abstract: form.abstract.trim(),
          content: form.content,
          comments_toggle: form.comments_toggle,
          category_id: form.category_id ?? null,
          tag_ids: [...form.tag_ids],
          cover_image_id: form.cover_image_id,
          cover: coverPreviewUrl.value || editingArticle.value.cover,
          publish_status: 1,
          status: 1,
          visibility_status: form.visibility_status,
        };
      }
      hydratedArticleId.value = editArticleId.value;
      coverDirty.value = false;
      message.success("当前文章已自动保存为草稿");
    }

    articleScoreData.value = await regenerateArticleScore({
      article_id: editArticleId.value,
      title: form.title.trim() || undefined,
      content: form.content.trim() || undefined,
    });
    message.success("已重新获取最新评分");
  } catch (error) {
    message.error(error instanceof Error ? error.message : "重新获取评分失败");
  } finally {
    articleScoreRefreshing.value = false;
  }
}

function openCoverFilePicker() {
  if (pendingState.cover) {
    return;
  }
  coverFileInputRef.value?.click();
}

function clearCoverSelection() {
  form.cover_image_id = null;
  coverPreviewUrl.value = "";
  coverUploadStage.value = "";
  coverDirty.value = true;
  if (coverFileInputRef.value) {
    coverFileInputRef.value.value = "";
  }
}

function mapCoverUploadStage(stage: "hashing" | "creating_task" | "uploading_to_qiniu" | "polling_status") {
  const textMap = {
    hashing: "正在计算文件指纹…",
    creating_task: "正在创建上传任务…",
    uploading_to_qiniu: "正在上传到对象存储…",
    polling_status: "正在确认图片状态…",
  } as const;
  coverUploadStage.value = textMap[stage];
}

function buildArticlePayload(status: 1 | 2) {
  const payload: {
    title: string;
    abstract?: string;
    content: string;
    category_id: string | null;
    tag_ids: string[];
    comments_toggle: boolean;
    visibility_status: "visible" | "user_hidden";
    status: 1 | 2;
    cover_image_id?: string | null;
  } = {
    title: form.title.trim(),
    abstract: form.abstract.trim() || undefined,
    content: form.content,
    category_id: form.category_id || null,
    tag_ids: form.tag_ids,
    comments_toggle: form.comments_toggle,
    visibility_status: form.visibility_status,
    status,
  };

  if (form.cover_image_id) {
    payload.cover_image_id = form.cover_image_id;
  } else if (isEditMode.value && coverDirty.value) {
    payload.cover_image_id = null;
  }

  return payload;
}

async function handleCoverFileChange(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) {
    return;
  }

  if (!file.type.startsWith("image/")) {
    message.warning("请选择图片文件作为封面。");
    input.value = "";
    return;
  }

  pendingState.cover = true;
  coverUploadStage.value = "开始上传封面…";
  try {
    const uploadResult = await uploadImageByTask(file, {
      onStage: mapCoverUploadStage,
    });
    if (!uploadResult.image_id || !uploadResult.url) {
      throw new Error("上传成功但未返回可用图片标识");
    }
    form.cover_image_id = uploadResult.image_id;
    coverPreviewUrl.value = uploadResult.url;
    coverDirty.value = true;
    coverUploadStage.value = "封面上传完成";
    message.success("封面上传成功");
  } catch (error) {
    coverUploadStage.value = "";
    message.error(error instanceof Error ? error.message : "封面上传失败");
  } finally {
    pendingState.cover = false;
    input.value = "";
  }
}

async function submitArticle(status: 1 | 2) {
  if (!canSubmit.value) {
    message.warning("标题和正文是必填项。");
    return;
  }

  if (pendingState.cover) {
    message.warning("封面仍在上传中，请稍后再提交。");
    return;
  }

  const key = status === 1 ? "draft" : "publish";
  pendingState[key] = true;

  try {
    const payload = buildArticlePayload(status);

    if (isEditMode.value && editArticleId.value) {
      await updateArticle(editArticleId.value, payload);
      publishVisible.value = false;
      if (status === 1) {
        message.success("草稿已更新");
      } else {
        message.success(
          form.visibility_status === "user_hidden" ? "文章已提交发布（当前为仅自己可见）" : "文章已提交发布（可能进入审核）",
        );
      }
      await router.push(`/article/${editArticleId.value}`);
      return;
    }

    const created = await createArticle(payload);
    publishVisible.value = false;
    if (status === 1) {
      message.success("草稿已保存");
    } else if (created.publish_status === 3) {
      message.success("文章已发布成功");
    } else if (created.publish_status === 2) {
      message.success("文章已提交审核，审核通过后会公开展示");
    } else {
      message.success("文章已提交发布");
    }
    await router.push(`/article/${created.id}`);
  } catch (error) {
    message.error(error instanceof Error ? error.message : "文章提交失败");
  } finally {
    pendingState[key] = false;
  }
}

useSeoMeta({
  title: "创作系统 - 写文章",
  description: "对齐原型工作台结构的正式创作页面。",
});
</script>

<template>
  <div class="write-page">
    <header class="write-page__header">
      <div class="write-page__topbar">
        <NInput v-model:value="form.title" class="write-title-input" size="large" maxlength="120"
          placeholder="输入文章标题..." :bordered="false" />

        <div class="write-page__actions">
          <NTooltip trigger="hover" placement="bottom">
            <template #trigger>
              <NButton quaternary @click="handleOpenArticleScore">质量评分</NButton>
            </template>
            {{ canOpenArticleScore ? "查看当前文章的历史评分，并可重新获取最新评分" : "请先保存为草稿，再发起文章质量评分" }}
          </NTooltip>
          <NButton quaternary :loading="pendingState.draft" @click="submitArticle(1)">{{ draftActionLabel }}</NButton>
          <NButton quaternary @click="navigateTo('/')">返回首页</NButton>
          <NButton quaternary @click="navigateTo('/studio/profile')">草稿箱</NButton>
          <NButton type="primary" @click="publishVisible = true">{{ isEditMode ? "更新" : "发布" }}</NButton>
          <NuxtLink :to="authStore.profileId ? `/users/${authStore.profileId}` : '/studio/profile'"
            class="write-avatar-link">
            <AppAvatar
              :key="authStore.profileAvatar || authStore.profileName"
              :src="authStore.profileAvatar"
              :name="authStore.profileName"
              :fallback="currentUserInitial" />
          </NuxtLink>
        </div>
      </div>

      <div class="write-page__toolbar">
        <div class="write-page__toolbar-group">
          <template v-for="tool in leftTools" :key="tool.key">
            <NPopover v-if="tool.options" trigger="hover" placement="bottom-start">
              <template #trigger>
                <button type="button" class="write-tool-button" :aria-label="tool.label" :title="getToolTooltip(tool)">
                  <component :is="tool.icon" class="write-tool-button__icon" :size="18" :stroke-width="1.9" />
                </button>
              </template>

              <div class="write-tool-popover">
                <button v-for="option in tool.options" :key="option.key" type="button" class="write-tool-option"
                  :class="{ 'write-tool-option--active': isToolOptionActive(tool.key, option.key) }"
                  @click.prevent.stop="handleToolOption(tool.key, option.key)">
                  <component v-if="option.icon" :is="option.icon" class="write-tool-option__icon" :size="16"
                    :stroke-width="1.9" />
                  {{ option.label }}
                </button>
              </div>
            </NPopover>

            <NTooltip v-else trigger="hover" placement="bottom">
              <template #trigger>
                <button type="button" class="write-tool-button" :aria-label="tool.label" :title="getToolTooltip(tool)"
                  @click="handleToolClick(tool)">
                  <component :is="tool.icon" class="write-tool-button__icon" :size="18" :stroke-width="1.9" />
                </button>
              </template>
              {{ getToolTooltip(tool) }}
            </NTooltip>
          </template>
        </div>

        <div class="write-page__toolbar-group write-page__toolbar-group--right">
          <NTooltip trigger="hover" placement="bottom">
            <template #trigger>
              <button type="button" class="write-tool-button" :class="{ 'write-tool-button--active': showToc }"
                aria-label="目录" @click="handleRightToolClick('toc')">
                <IconListDetails class="write-tool-button__icon" :size="18" :stroke-width="1.9" />
              </button>
            </template>
            目录
          </NTooltip>

          <NTooltip trigger="hover" placement="bottom">
            <template #trigger>
              <button type="button" class="write-tool-button"
                :class="{ 'write-tool-button--active': viewMode === 'editor' }" aria-label="仅显示编辑区"
                @click="handleRightToolClick('editor')">
                <IconColumns1 class="write-tool-button__icon" :size="18" :stroke-width="1.9" />
              </button>
            </template>
            仅显示编辑区
          </NTooltip>

          <NTooltip trigger="hover" placement="bottom">
            <template #trigger>
              <button type="button" class="write-tool-button"
                :class="{ 'write-tool-button--active': viewMode === 'preview' }" aria-label="仅显示预览区"
                @click="handleRightToolClick('preview')">
                <IconColumns2 class="write-tool-button__icon" :size="18" :stroke-width="1.9" />
              </button>
            </template>
            仅显示预览区
          </NTooltip>
        </div>
      </div>
    </header>

    <main class="write-page__body" :class="writeBodyClass">
      <aside v-show="showToc" class="write-page__toc">
        <div class="write-page__toc-header">目录</div>
        <nav class="write-page__toc-list">
          <button v-for="item in tocItems" :key="`${item.level}-${item.label}`" type="button"
            class="write-page__toc-item" :class="[`write-page__toc-item--level-${Math.min(item.level, 4)}`]"
            @click="handleTocJump(item.id)">
            {{ item.label }}
          </button>
        </nav>
      </aside>

      <section class="write-page__pane write-page__pane--editor">
        <textarea ref="editorTextareaRef" v-model="form.content" class="write-page__textarea"
          @keydown="handleEditorShortcuts"
          @keyup="handleEditorSelectionChange"
          @mouseup="handleEditorSelectionChange"
          @select="handleEditorSelectionChange"
          @scroll="handleEditorSelectionChange"
          placeholder="从这里开始写作，把 API 能力、页面结构和组件关系整理清楚。" />
      </section>

      <aside class="write-page__pane write-page__pane--preview">
        <div v-if="form.content.trim()" class="write-page__preview-shell">
          <div class="write-page__theme-indicator">预览样式：{{ selectedThemeLabel }}</div>
          <WriteMarkdownRenderer
            ref="shadowPreviewRef"
            class="write-page__markdown"
            :source="debouncedContent"
            :theme-href="activeMarkdownThemeHref"
            :extra-style-hrefs="markdownSupportStyleHrefs"
            article-class="markdown-body"
            @headings-change="handlePreviewHeadingsChange" />
        </div>

        <div v-else class="write-page__preview-empty">
          <p>预览区</p>
          <span>先输入标题或正文，这里会实时显示正式渲染结果。</span>
        </div>
      </aside>
    </main>

    <footer class="write-page__statusbar">
      <span>字符数: {{ contentStats.raw }}</span>
      <span>正文字符数: {{ contentStats.pure }}</span>
      <span>行数: {{ contentStats.lines }}</span>
      <span>目录项: {{ previewHeadings.length }}</span>
      <span>视图: {{ viewMode === 'split' ? '双栏' : viewMode === 'editor' ? '仅编辑区' : '仅预览区' }}</span>
    </footer>

    <StudioSelectionAiToolbar
      :show="selectionOverlay.visible"
      :top="selectionOverlay.top"
      :left="selectionOverlay.left"
      @action="handleSelectionToolbarAction" />

    <StudioAiAssistPanel
      :show="aiPanelVisible"
      :action="aiPanelAction"
      :selection="aiSelectionContext"
      :style-instruction="aiStyleInstruction"
      :overwrite-result="aiOverwriteResult"
      :diagnose-result="aiDiagnoseResult"
      :pending="aiPanelBusy"
      :error-message="aiErrorMessage"
      :can-submit="aiCanSubmit"
      @update:show="handleAiPanelVisibility"
      @update:style-instruction="aiStyleInstruction = $event"
      @run="handleAiPanelRun"
      @replace="handleAiReplace"
      @insert-below="handleAiInsertBelow"
      @copy="handleAiCopyResult" />

    <ArticleScoreModal
      v-model:show="articleScoreModalOpen"
      title="文章质量评分"
      :loading="articleScoreLoading"
      :refreshing="articleScoreRefreshing"
      :data="articleScoreData"
      :allow-refresh="Boolean(editArticleId)"
      refresh-label="重新获取（自动保存文章为草稿）"
      empty-text="当前还没有历史评分缓存，可以点击“重新获取”生成最新评分。"
      @refresh="handleRefreshArticleScore" />

    <NModal v-model:show="publishVisible" :mask-closable="!isPublishingLocked">
      <div class="publish-modal-shell">
        <NCard title="发布文章" :bordered="false" closable class="publish-modal-card" @close="publishVisible = false">
          <div class="publish-form">
            <div class="publish-form__row">
              <label class="publish-form__label">分类</label>
              <NSelect v-model:value="form.category_id" clearable filterable :options="categoryOptions || []"
                placeholder="请选择分类" class="publish-form__control" />
            </div>

            <div class="publish-form__row">
              <label class="publish-form__label">添加标签</label>
              <NSelect v-model:value="form.tag_ids" multiple clearable filterable max-tag-count="responsive"
                :options="tagOptions || []" placeholder="请选择已有标签" class="publish-form__control" />
            </div>

            <div class="publish-form__row publish-form__row--top">
              <label class="publish-form__label">文章封面</label>
              <div class="publish-cover">
                <input
                  ref="coverFileInputRef"
                  type="file"
                  accept="image/*"
                  class="publish-cover__file-input"
                  @change="handleCoverFileChange" />

                <button
                  type="button"
                  class="publish-cover__picker"
                  :class="{ 'publish-cover__picker--busy': pendingState.cover }"
                  @click="openCoverFilePicker">
                  <img
                    v-if="coverPreviewUrl"
                    :src="coverPreviewUrl"
                    alt="文章封面预览"
                    class="publish-cover__preview-image" />
                  <template v-else>
                    <span class="publish-cover__plus">+</span>
                    <span>上传封面</span>
                  </template>
                </button>

                <div class="publish-cover__actions">
                  <NButton quaternary size="small" :loading="pendingState.cover" @click="openCoverFilePicker">
                    {{ coverPreviewUrl ? "重新上传" : "选择图片" }}
                  </NButton>
                  <NButton v-if="coverPreviewUrl || form.cover_image_id" quaternary size="small" @click="clearCoverSelection">
                    移除封面
                  </NButton>
                </div>
                <p v-if="coverUploadStage" class="publish-cover__stage">{{ coverUploadStage }}</p>
              </div>
            </div>

            <div class="publish-form__row publish-form__row--top">
              <label class="publish-form__label">编辑摘要</label>
              <div class="publish-summary">
                <NInput v-model:value="form.abstract" type="textarea" maxlength="180"
                  :autosize="{ minRows: 5, maxRows: 8 }" placeholder="请输入文章摘要" />
                <span class="publish-summary__count">{{ form.abstract.length }}/180</span>
              </div>
            </div>

            <div class="publish-form__row">
              <label class="publish-form__label">可见范围</label>
              <NSelect
                v-model:value="form.visibility_status"
                :options="visibilityOptions"
                placeholder="请选择文章可见范围"
                class="publish-form__control" />
            </div>

            <div class="publish-form__row">
              <label class="publish-form__label">评论开关</label>
              <div class="publish-form__switch">
                <span class="muted">{{ form.comments_toggle ? "文章发布后允许评论" : "文章发布后将关闭评论区" }}</span>
                <NSwitch v-model:value="form.comments_toggle" />
              </div>
            </div>

          </div>

          <template #footer>
            <div class="publish-modal__footer">
              <NButton quaternary @click="publishVisible = false">取消</NButton>
              <NButton secondary :loading="pendingState.ai" @click="handleAiAssist()">AI 填入</NButton>
              <NButton quaternary :loading="pendingState.draft" :disabled="pendingState.cover" @click="submitArticle(1)">
                {{ draftActionLabel }}
              </NButton>
              <NButton type="primary" :loading="pendingState.publish" :disabled="pendingState.cover" @click="submitArticle(2)">
                {{ publishActionLabel }}
              </NButton>
            </div>
          </template>
        </NCard>
      </div>
    </NModal>
  </div>
</template>

<style scoped>
.write-page {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}

.write-page__header {
  position: sticky;
  top: 0;
  z-index: 20;
  backdrop-filter: blur(18px);
  background: rgba(255, 251, 245, 0.9);
  border-bottom: 1px solid rgba(217, 226, 236, 0.92);
}

.write-page__topbar,
.write-page__toolbar,
.write-page__statusbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 14px 24px;
}

.write-page__toolbar,
.write-page__statusbar {
  border-top: 1px solid rgba(217, 226, 236, 0.72);
}

.write-title-input {
  flex: 1;
}

.write-title-input :deep(.n-input) {
  background: transparent;
}

.write-title-input :deep(.n-input__input-el) {
  font-size: 24px;
  font-weight: 600;
  letter-spacing: -0.03em;
}

.write-page__actions {
  display: flex;
  align-items: center;
  gap: 12px;
  white-space: nowrap;
}

.write-avatar-link {
  display: inline-flex;
}

.write-page__toolbar-group {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.write-tool-button {
  min-width: 34px;
  min-height: 34px;
  border: 1px solid transparent;
  border-radius: 10px;
  padding: 6px 8px;
  background: transparent;
  color: #334155;
  cursor: pointer;
  transition: background 0.2s ease, color 0.2s ease, border-color 0.2s ease;
}

.write-tool-button__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  font-size: 16px;
  line-height: 1;
}

.write-tool-button:hover {
  background: rgba(15, 118, 110, 0.08);
  border-color: rgba(15, 118, 110, 0.12);
  color: #0f766e;
}

.write-tool-button--active {
  background: rgba(15, 118, 110, 0.12);
  border-color: rgba(15, 118, 110, 0.2);
  color: #0f766e;
}

.write-tool-popover {
  min-width: 148px;
  max-height: 340px;
  display: flex;
  flex-direction: column;
  padding: 8px 0;
  overflow: auto;
}

.write-tool-option {
  display: flex;
  align-items: center;
  gap: 10px;
  border: 0;
  padding: 10px 14px;
  text-align: left;
  background: transparent;
  color: #334155;
  cursor: pointer;
  transition: background 0.18s ease, color 0.18s ease;
}

.write-tool-option:hover {
  background: rgba(15, 118, 110, 0.08);
  color: #0f766e;
}

.write-tool-option--active {
  background: rgba(15, 118, 110, 0.16);
  color: #0f766e;
  font-weight: 600;
}

.write-page__body {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 200px minmax(0, 1fr) minmax(340px, 1fr);
}

.write-page__body--hide-toc {
  grid-template-columns: minmax(0, 1fr) minmax(340px, 1fr);
}

.write-page__body--editor-only {
  grid-template-columns: 200px minmax(0, 1fr);
}

.write-page__body--editor-only.write-page__body--hide-toc,
.write-page__body--preview-only.write-page__body--hide-toc {
  grid-template-columns: minmax(0, 1fr);
}

.write-page__body--preview-only {
  grid-template-columns: 200px minmax(0, 1fr);
}

.write-page__pane {
  min-height: 0;
  overflow: auto;
}

.write-page__toc {
  border-right: 1px solid rgba(217, 226, 236, 0.92);
  background:
    linear-gradient(180deg, rgba(245, 248, 252, 0.96) 0%, rgba(241, 245, 249, 0.86) 100%);
}

.write-page__toc-header {
  padding: 18px 16px 10px;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #475569;
}

.write-page__toc-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 0 10px 14px;
}

.write-page__toc-item {
  border: 1px solid transparent;
  border-radius: 10px;
  padding: 8px 10px;
  text-align: left;
  color: #334155;
  background: rgba(255, 255, 255, 0.62);
  cursor: pointer;
  font-size: 13px;
  line-height: 1.35;
  transition:
    background 0.18s ease,
    color 0.18s ease,
    border-color 0.18s ease,
    transform 0.18s ease;
}

.write-page__toc-item:hover {
  transform: translateX(1px);
  background: rgba(15, 118, 110, 0.1);
  border-color: rgba(15, 118, 110, 0.18);
  color: #0f766e;
}

.write-page__toc-item--level-2 {
  padding-left: 24px;
  color: #475569;
}

.write-page__toc-item--level-3 {
  padding-left: 36px;
  color: #64748b;
}

.write-page__toc-item--level-4 {
  padding-left: 48px;
  color: #94a3b8;
}

.write-page__pane--editor {
  border-right: 1px solid rgba(217, 226, 236, 0.92);
}

.write-page__body--editor-only .write-page__pane--preview,
.write-page__body--preview-only .write-page__pane--editor {
  display: none;
}

.write-page__textarea {
  width: 100%;
  height: 100%;
  min-height: calc(100vh - 154px);
  border: 0;
  resize: none;
  padding: 24px 32px 40px;
  font: inherit;
  font-size: 17px;
  line-height: 1.9;
  color: #102031;
  background: transparent;
  outline: none;
}

.write-page__preview-shell {
  min-height: 100%;
  padding: 32px 36px 48px;
}

.write-page__theme-indicator {
  display: inline-flex;
  align-items: center;
  margin-bottom: 14px;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 12px;
  color: #0f766e;
  background: rgba(15, 118, 110, 0.08);
}

.write-page__markdown {
  max-width: none;
  min-height: 100%;
  box-sizing: border-box;
}

.write-page__preview-empty {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: #6b7f95;
}

.write-page__preview-empty p {
  margin: 0;
  font-size: 18px;
  color: #334155;
}

.write-page__statusbar {
  justify-content: flex-start;
  gap: 18px;
  color: #6b7f95;
  font-size: 13px;
  background: rgba(255, 251, 245, 0.84);
}

.publish-modal-shell {
  width: min(760px, calc(100vw - 96px));
  margin: 0 auto;
}

.publish-modal-card {
  border-radius: 20px;
}

.publish-form {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.publish-form__row {
  display: grid;
  grid-template-columns: 84px minmax(0, 1fr);
  align-items: center;
  gap: 18px;
}

.publish-form__row--top {
  align-items: flex-start;
}

.publish-form__label {
  font-size: 14px;
  color: #334155;
}

.publish-form__control {
  width: 100%;
}

.publish-cover {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.publish-cover__file-input {
  display: none;
}

.publish-cover__picker {
  width: 194px;
  height: 130px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  border: 1px dashed rgba(148, 163, 184, 0.7);
  border-radius: 16px;
  color: #64748b;
  background: rgba(248, 250, 252, 0.8);
  cursor: pointer;
  overflow: hidden;
}

.publish-cover__picker--busy {
  opacity: 0.72;
  cursor: wait;
}

.publish-cover__preview-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.publish-cover__plus {
  font-size: 28px;
  line-height: 1;
}

.publish-cover__actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.publish-cover__stage {
  margin: 0;
  font-size: 12px;
  color: #0f766e;
}

.publish-summary {
  position: relative;
}

.publish-summary__count {
  position: absolute;
  right: 12px;
  bottom: 10px;
  font-size: 12px;
  color: #f97316;
}

.publish-form__switch {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 16px;
  border: 1px solid rgba(217, 226, 236, 0.82);
  border-radius: 16px;
  background: rgba(248, 250, 252, 0.8);
}

.publish-modal__footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

@media (max-width: 1120px) {

  .write-page__topbar,
  .write-page__toolbar,
  .write-page__statusbar {
    padding: 12px 16px;
  }

  .write-page__actions {
    gap: 10px;
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .write-page__toolbar-group--right,
  .write-page__toc,
  .write-page__pane--preview {
    display: none !important;
  }

  .write-page__body,
  .write-page__body--hide-toc,
  .write-page__body--editor-only,
  .write-page__body--preview-only,
  .write-page__body--editor-only.write-page__body--hide-toc,
  .write-page__body--preview-only.write-page__body--hide-toc {
    grid-template-columns: 1fr !important;
  }

  .write-page__pane--editor {
    border-right: 0;
    border-bottom: 0;
  }

  .write-page__textarea {
    min-height: 420px;
    padding: 20px 18px 28px;
  }

  .write-page__preview-shell {
    padding: 24px 18px 32px;
  }
}

@media (max-width: 720px) {
  .write-title-input :deep(.n-input__input-el) {
    font-size: 20px;
  }

  .write-page__actions .muted {
    display: none;
  }

  .publish-form__row {
    grid-template-columns: 1fr;
    gap: 10px;
  }

  .publish-modal__footer {
    flex-wrap: wrap;
  }

  .publish-modal-shell {
    width: calc(100vw - 28px) !important;
  }
}
</style>
