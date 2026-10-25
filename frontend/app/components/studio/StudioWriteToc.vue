<script setup lang="ts">
// 写作页的目录栏。
//
// 从 pages/studio/write.vue 拆出来（M6 ⑧）。它是纯展示组件：
// 接收标题锚点列表、上抛"跳到某标题"。不持有任何状态 ——
// 滚动到哪由预览组件自己负责，目录只负责"点了哪个 id"。
//
// 注意样式仍留在页面里（.write-page__toc*）。组件根元素会带上父组件的
// scoped id，所以外层规则照旧生效；内部元素的选择器在页面里加了 :deep()。

export type TocItem = {
  id: string;
  label: string;
  level: number;
};

defineProps<{
  /** 是否显示（页面用它做"折叠目录"）。 */
  visible: boolean;
  items: TocItem[];
}>();

const emit = defineEmits<{ jump: [id: string] }>();
</script>

<template>
  <aside v-show="visible" class="write-page__toc">
    <div class="write-page__toc-header">目录</div>
    <nav class="write-page__toc-list">
      <button
        v-for="item in items"
        :key="`${item.level}-${item.label}`"
        type="button"
        class="write-page__toc-item"
        :class="[`write-page__toc-item--level-${Math.min(item.level, 4)}`]"
        @click="emit('jump', item.id)"
      >
        {{ item.label }}
      </button>
    </nav>
  </aside>
</template>
