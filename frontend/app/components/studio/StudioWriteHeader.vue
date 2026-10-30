<script setup lang="ts">
import { NButton, NInput, NPopover, NTooltip } from "naive-ui";

import { LEFT_TOOLS, getToolTooltip, type ToolItem } from "~/constants/studioToolbar";
import { IconColumns1, IconColumns2, IconListDetails } from "@tabler/icons-vue";
import AppAvatar from "~/components/common/AppAvatar.vue";

// 写作页顶部：标题输入 + 操作按钮 + 工具栏。
//
// 从 pages/studio/write.vue 拆出来（M6 ⑧）。拆的依据是**数据流方向**：
// 这一段是纯"展示 + 上抛事件"，自己不持有任何业务状态 ——
// 标题通过 v-model 双向绑定，按钮点击一律 emit 出去。
//
// 这样页面不需要知道工具栏渲染了几个按钮，组件也不需要知道"保存草稿"背后
// 调了哪个接口。

const title = defineModel<string>("title", { required: true });

const props = defineProps<{
  /** 草稿保存中。 */
  draftPending: boolean;
  /** 草稿按钮文案（新建/编辑不同）。 */
  draftLabel: string;
  /** 是否编辑已存在的文章。 */
  isEditMode: boolean;
  /** 是否已有可评分的文章。 */
  canOpenScore: boolean;
  /** 目录栏是否展开。 */
  showToc: boolean;
  /** 当前视图模式。 */
  viewMode: "split" | "editor" | "preview";
  /** 用户 id（为空时头像链到草稿箱）。 */
  profileId: string;
  /** 头像地址。 */
  profileAvatar?: string;
  /** 用户昵称。 */
  profileName?: string;
  /** 头像兜底字符。 */
  profileInitial: string;
  /** 判断某个下拉选项是否处于选中态（主题等）。 */
  isOptionActive: (toolKey: string, optionKey: string) => boolean;
}>();

const emit = defineEmits<{
  openScore: [];
  saveDraft: [];
  openPublish: [];
  toolClick: [tool: ToolItem];
  toolOption: [toolKey: string, optionKey: string];
  rightTool: [key: "toc" | "editor" | "preview"];
}>();

function handleToolClick(tool: ToolItem) {
  emit("toolClick", tool);
}

function handleToolOption(toolKey: string, optionKey: string) {
  emit("toolOption", toolKey, optionKey);
}

function handleRightToolClick(key: "toc" | "editor" | "preview") {
  emit("rightTool", key);
}
</script>

<template>
<header class="write-page__header">
<div class="write-page__topbar">
<NInput
v-model:value="title" class="write-title-input" size="large" maxlength="120"
placeholder="输入文章标题..." :bordered="false" />

<div class="write-page__actions">
<NTooltip trigger="hover" placement="bottom">
<template #trigger>
<NButton quaternary @click="emit('openScore')">质量评分</NButton>
</template>
{{ props.canOpenScore ? "查看当前文章的历史评分，并可重新获取最新评分" : "请先保存为草稿，再发起文章质量评分" }}
</NTooltip>
<NButton quaternary :loading="props.draftPending" @click="emit('saveDraft')">{{ props.draftLabel }}</NButton>
<NButton quaternary @click="navigateTo('/')">返回首页</NButton>
<NButton quaternary @click="navigateTo('/studio/profile')">草稿箱</NButton>
<NButton type="primary" @click="emit('openPublish')">{{ props.isEditMode ? "更新" : "发布" }}</NButton>
<NuxtLink
:to="props.profileId ? `/users/${props.profileId}` : '/studio/profile'"
class="write-avatar-link">
<AppAvatar
:key="props.profileAvatar || props.profileName"
:src="props.profileAvatar"
:name="props.profileName"
:fallback="props.profileInitial" />
</NuxtLink>
</div>
</div>

<div class="write-page__toolbar">
<div class="write-page__toolbar-group">
<template v-for="tool in LEFT_TOOLS" :key="tool.key">
<NPopover v-if="tool.options" trigger="hover" placement="bottom-start">
<template #trigger>
<button type="button" class="write-tool-button" :aria-label="tool.label" :title="getToolTooltip(tool)">
<component :is="tool.icon" class="write-tool-button__icon" :size="18" :stroke-width="1.9" />
</button>
</template>

<div class="write-tool-popover">
<button
v-for="option in tool.options" :key="option.key" type="button" class="write-tool-option"
:class="{ 'write-tool-option--active': props.isOptionActive(tool.key, option.key) }"
@click.prevent.stop="handleToolOption(tool.key, option.key)">
<component
:is="option.icon" v-if="option.icon" class="write-tool-option__icon" :size="16"
:stroke-width="1.9" />
{{ option.label }}
</button>
</div>
</NPopover>

<NTooltip v-else trigger="hover" placement="bottom">
<template #trigger>
<button
type="button" class="write-tool-button" :aria-label="tool.label" :title="getToolTooltip(tool)"
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
<button
type="button" class="write-tool-button" :class="{ 'write-tool-button--active': props.showToc }"
aria-label="目录" @click="handleRightToolClick('toc')">
<IconListDetails class="write-tool-button__icon" :size="18" :stroke-width="1.9" />
</button>
</template>
目录
</NTooltip>

<NTooltip trigger="hover" placement="bottom">
<template #trigger>
<button
type="button" class="write-tool-button"
:class="{ 'write-tool-button--active': props.viewMode === 'editor' }" aria-label="仅显示编辑区"
@click="handleRightToolClick('editor')">
<IconColumns1 class="write-tool-button__icon" :size="18" :stroke-width="1.9" />
</button>
</template>
仅显示编辑区
</NTooltip>

<NTooltip trigger="hover" placement="bottom">
<template #trigger>
<button
type="button" class="write-tool-button"
:class="{ 'write-tool-button--active': props.viewMode === 'preview' }" aria-label="仅显示预览区"
@click="handleRightToolClick('preview')">
<IconColumns2 class="write-tool-button__icon" :size="18" :stroke-width="1.9" />
</button>
</template>
仅显示预览区
</NTooltip>
</div>
</div>
</header>
</template>

<style scoped>
/* 颜色一律消费语义 token（票 #9 把创作区从旧青绿色板收敛到新体系）。 */
.write-page__header {
  position: sticky;
  top: 0;
  z-index: 20;
  backdrop-filter: blur(18px);
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-line);
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
  border-top: 1px solid var(--color-line);
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
  border-radius: var(--radius-sm);
  padding: 6px 8px;
  background: transparent;
  color: var(--color-ink);
  cursor: pointer;
  transition:
    background var(--motion-fast) var(--motion-ease),
    color var(--motion-fast) var(--motion-ease),
    border-color var(--motion-fast) var(--motion-ease);
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
  background: var(--color-line-faint);
  border-color: var(--color-line);
  color: var(--color-ink);
}

.write-tool-button--active {
  background: var(--color-line-faint);
  border-color: var(--color-line);
  color: var(--color-ink);
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
  color: var(--color-ink);
  cursor: pointer;
  transition:
    background var(--motion-fast) var(--motion-ease),
    color var(--motion-fast) var(--motion-ease);
}

.write-tool-option:hover {
  background: var(--color-line-faint);
  color: var(--color-ink);
}

.write-tool-option--active {
  background: var(--color-line-faint);
  color: var(--color-ink);
  font-weight: 600;
}

</style>
