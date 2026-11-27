<script setup lang="ts">
/**
 * 关键词过滤的两页共用分页条（词表 / 白名单 / 保留期）。
 *
 * 为什么抽成组件：词表页与白名单页各自都要这一排 tabs，「白名单」是跳到另一页的
 * **链接**（不是 tab），「保留期」在词表页属于本页、在白名单页则要跳回词表页
 * `?tab=retention`。两边各写一遍的话，只要有一边忘改，同一个首页上就会有两套
 * 分页语义 —— 所以只留这一份。
 *
 * `crossPage=true`（白名单页）时，词表与保留期都渲染成链接；
 * `crossPage=false`（词表页）时它们是本地 tab，点击只切视图、不产生路由历史。
 */
import { computed } from "vue";

const props = defineProps<{
  active: "words" | "whitelist" | "retention";
  wordCount: number;
  whitelistCount: number;
  /** true = 本页只显示白名单，词表/保留期要跳回词表页。 */
  crossPage?: boolean;
}>();

const emit = defineEmits<{ (event: "select", tab: "words" | "retention"): void }>();

const retentionHref = computed(() => ({ path: "/content/keywords", query: { tab: "retention" } }));
</script>

<template>
  <nav class="cf-tabs" role="tablist" aria-label="关键词过滤分页">
    <span
      v-if="active === 'words'"
      class="cf-tab is-active"
      role="tab"
      aria-selected="true"
    >
      词表（{{ wordCount }}）
    </span>
    <NuxtLink
      v-else-if="props.crossPage"
      class="cf-tab"
      role="tab"
      aria-selected="false"
      to="/content/keywords"
    >
      词表（{{ wordCount }}）
    </NuxtLink>
    <button
      v-else
      class="cf-tab"
      type="button"
      role="tab"
      aria-selected="false"
      @click="emit('select', 'words')"
    >
      词表（{{ wordCount }}）
    </button>

    <NuxtLink
      class="cf-tab"
      :class="{ 'is-active': active === 'whitelist' }"
      role="tab"
      :aria-selected="active === 'whitelist'"
      to="/content/keywords/whitelist"
    >
      白名单（{{ whitelistCount }}）
    </NuxtLink>

    <span
      v-if="active === 'retention'"
      class="cf-tab is-active"
      role="tab"
      aria-selected="true"
    >
      保留期
    </span>
    <NuxtLink
      v-else-if="props.crossPage"
      class="cf-tab"
      role="tab"
      aria-selected="false"
      :to="retentionHref"
    >
      保留期
    </NuxtLink>
    <button
      v-else
      class="cf-tab"
      type="button"
      role="tab"
      aria-selected="false"
      @click="emit('select', 'retention')"
    >
      保留期
    </button>
  </nav>
</template>
