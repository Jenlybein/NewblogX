<script setup lang="ts">
import { computed } from "vue";
import OutlineTree from "~/components/outline/OutlineTree.vue";
import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";
import { useOutlineCollapsed } from "~/composables/useOutlineCollapsed";
import { getProjectDetail } from "~/services/project";
import { projectOutlineView } from "~/utils/project";

/**
 * 项目详情（票 219：详情页 = 目录页，决定清单 A7–A8；项目入口文案一律写「项目」）。
 *
 * 渲染边界：不进 SSR 白名单（与列表页、合集、搜索页同一处理，ADR-0019）。
 *
 * 后端返回的是与合集同构的嵌套目录树；文件夹可展开 / 收起、文章节点带
 * `?from=project:<id>` 打开阅读页，层级只由缩进表达（不再有「第 N 篇」）。
 * 折叠状态按树存在 localStorage，刷新或返回后保持。
 *
 * 未展示的项目后端按**不存在**返回错误：这里把错误渲染成中性空态，而不是
 * "0 篇文章"——后者会让读者以为项目是空的，也容易把内部状态说破。
 * 树里的失效行后端已过滤；封面回退由后端算好，前端不另写一份。
 */
const route = useRoute();
const projectId = computed(() => String(route.params.id ?? ""));

// 折叠状态不依赖取数，先注册：async setup 里晚注册的生命周期钩子容易漏掉。
const { collapsed, toggle } = useOutlineCollapsed("project", projectId.value);

const { data, pending, error } = await useAsyncData(
  () => `reader-project-${projectId.value}`,
  () => getProjectDetail(projectId.value),
  { watch: [projectId] },
);

useSeoMeta({
  title: computed(() => data.value?.title || "项目"),
  description: computed(() => data.value?.description || "按项目成组阅读的文章。"),
});

const outlineView = computed(() => projectOutlineView(data.value));
const outlineNodes = computed(() => outlineView.value.nodes);
const articleCount = computed(() => outlineView.value.articleCount);
const description = computed(() => (data.value?.description ?? "").trim());
const emptyTitle = computed(() => (error.value ? "项目不存在" : "这个项目还没有文章"));
const emptyDescription = computed(() =>
  error.value ? "这个项目暂时无法访问。" : "管理员还没有把文章收录进这个项目。",
);
</script>

<template>
  <main id="main-content" class="page-shell project-detail">
    <header class="project-detail__header">
      <NuxtLink to="/projects" class="project-detail__back">← 返回项目</NuxtLink>

      <h1 class="project-detail__title">{{ data?.title || "项目" }}</h1>
      <p v-if="description" class="project-detail__lede">{{ description }}</p>
      <p v-if="articleCount" class="project-detail__meta">共 {{ articleCount }} 篇</p>
    </header>

    <OutlineTree
      v-if="outlineNodes.length"
      class="project-detail__outline"
      data-project-outline="true"
      :nodes="outlineNodes"
      kind="project"
      :owner-id="projectId"
      :collapsed="collapsed"
      @toggle="toggle"
    />

    <ProfileEmptyState
      v-else
      :title="emptyTitle"
      :description="emptyDescription"
      :pending="pending"
      size="lg"
    />
  </main>
</template>

<style scoped>
.project-detail {
  padding-top: 2rem;
  padding-bottom: 4rem;
}

.project-detail__header {
  max-width: 44rem;
  margin-bottom: 2rem;
}

/* 显式带上 `a` 前缀：链接色只允许用在文字链接上（设计门禁按选择器判定）。 */
a.project-detail__back {
  display: inline-block;
  margin-bottom: 1rem;
  color: var(--color-link);
  font-size: 0.875rem;
  text-decoration: none;
}

a.project-detail__back:hover {
  text-decoration: underline;
}

.project-detail__title {
  margin: 0 0 0.75rem;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: clamp(1.75rem, 4vw, 2.5rem);
  line-height: 1.15;
  letter-spacing: -0.02em;
}

.project-detail__lede {
  margin: 0 0 0.75rem;
  color: var(--color-ink-muted);
  font-size: 0.9375rem;
  line-height: 1.7;
}

.project-detail__meta {
  margin: 0;
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.75rem;
  letter-spacing: 0.08em;
}

.project-detail__outline {
  max-width: 48rem;
}

.project-detail :is(a, button, [tabindex]):focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}
</style>
