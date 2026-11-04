<script setup lang="ts">
/**
 * 合集 / 项目的封面卡片网格（决定清单 §8 第 33–34 条）。
 *
 * 卡片只呈现：封面（没有就回退首篇，两张都没有时显式占位，绝不发空 `src`）、
 * 标题、描述、篇数、展示开关。**编辑 / 删除只在 hover 时浮出**，卡面本身不被按钮占满。
 *
 * 组件保持"哑"：不发请求、不持状态，点击只抛事件（两个页面共用同一份）。
 */
import { computed } from "vue";
import { NButton, NSwitch } from "naive-ui";
import { resolveCollectionCover } from "~/utils/collectionAdmin";

/** 合集与项目的卡片在字段上同形，这里只取渲染用得到的部分。 */
export interface CollectionCardItem {
  id: string;
  title: string;
  description: string;
  cover: string;
  fallback_cover: string;
  article_count: number;
  displayed: boolean;
}

const props = withDefaults(
  defineProps<{
    kind: "collection" | "project";
    items: readonly CollectionCardItem[];
    loading?: boolean;
    /** 正在切换展示开关的那张卡（同一时间只允许一条 PATCH 在途）。 */
    togglingId?: string;
  }>(),
  { loading: false, togglingId: "" },
);

const emit = defineEmits<{
  edit: [CollectionCardItem];
  remove: [CollectionCardItem];
  toggle: [CollectionCardItem, boolean];
}>();

const label = computed(() => (props.kind === "collection" ? "合集" : "项目"));

function coverOf(item: CollectionCardItem) {
  return resolveCollectionCover(item.cover, item.fallback_cover);
}
</script>

<template>
  <div class="collection-grid" :data-role="`${kind}-card-grid`">
    <p v-if="!props.items.length && !props.loading" class="admin-hint" :data-role="`${kind}-grid-empty`">
      还没有{{ label }}。点右上角「新建{{ label }}」开始。
    </p>

    <article
      v-for="item in props.items"
      :key="item.id"
      class="collection-card"
      :data-role="`${kind}-card`"
      :data-id="item.id"
    >
      <div class="collection-card__cover">
        <img v-if="coverOf(item)" :src="coverOf(item)!.url" :alt="item.title || label" />
        <span v-else class="collection-card__cover-empty">无封面</span>

        <!-- hover / 键盘聚焦才浮出：卡片默认只有内容。 -->
        <div class="collection-card__hover">
          <NButton size="small" type="primary" :data-role="`${kind}-card-edit`" @click="emit('edit', item)">
            编辑
          </NButton>
          <NButton size="small" type="error" :data-role="`${kind}-card-remove`" @click="emit('remove', item)">
            删除
          </NButton>
        </div>
      </div>

      <div class="collection-card__body">
        <div class="collection-card__head">
          <strong class="collection-card__title" :title="item.title">{{ item.title || "（无标题）" }}</strong>
          <NSwitch
            size="small"
            :value="item.displayed"
            :loading="props.togglingId === item.id"
            :aria-label="item.displayed ? '当前展示中，点击隐藏' : '当前已隐藏，点击展示'"
            @update:value="(value: boolean) => emit('toggle', item, value)" />
        </div>
        <p class="collection-card__desc">{{ item.description || "暂无描述" }}</p>
        <span class="collection-card__meta">{{ item.article_count }} 篇</span>
      </div>
    </article>
  </div>
</template>
