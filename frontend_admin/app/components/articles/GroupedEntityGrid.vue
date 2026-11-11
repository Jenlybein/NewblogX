<script setup lang="ts">
import { MOTION_EXPRESSIVE_MS } from '~/utils/motion'
/**
 * 合集 / 项目分组网格（票 348，两个页面共用一份）。
 *
 * 结构（与 ADR-0041 的读面一一对应）：
 *
 *   ┌ 未分组 ────────────────────────────────┐  ← 固定第一，空时整段消失
 *   │  [卡] [卡] [卡]                        │
 *   ├ 分组标题（可拖动）──────────────────────┤
 *   │  [卡] [卡]                             │
 *   └────────────────────────────────────────┘
 *
 * 三条拖拽语义：
 *  1. **卡拖进别的分组** = 改归属（跨组移动）；
 *  2. **卡在组内拖动** = 改组内顺序；
 *  3. **拖分组标题** = 改分组之间的顺序；拖动期间把各组内容临时收起，
 *     否则长列表下"拖到哪儿了"完全看不出来。
 *
 * 组件保持"哑"：不发请求、不持保存状态，顺序变化只抛 `orderChange`
 * （整树快照），由页面用 useSortPersist 做 10 秒防抖 + 离开前 flush。
 */
import { computed, ref } from 'vue'
import { NButton, NSwitch } from 'naive-ui'
import { VueDraggable } from 'vue-draggable-plus'
import { resolveCollectionCover } from '~/utils/collectionAdmin'

/** 与 CollectionCardGrid 同一份卡片字段（两个组件消费同一批数据）。 */
export interface GroupedCardItem {
  id: string
  title: string
  description: string
  cover: string
  fallback_cover: string
  article_count: number
  displayed: boolean
}

/** 一个分组在界面上的形态：标题 + 卡片的当前排列。 */
export interface GroupedGroup {
  id: string
  title: string
  items: GroupedCardItem[]
}

const props = withDefaults(defineProps<{
  kind: 'collection' | 'project'
  groups: GroupedGroup[]
  ungrouped: GroupedCardItem[]
  loading?: boolean
  /** 正在切换展示开关的那张卡（同一时间只允许一条 PATCH 在途）。 */
  togglingId?: string
  /** 正在保存顺序（页面把 useSortPersist 的状态透进来）。 */
  savingOrder?: boolean
}>(), {
  loading: false,
  togglingId: '',
  savingOrder: false,
})

const emit = defineEmits<{
  edit: [GroupedCardItem]
  remove: [GroupedCardItem]
  toggle: [GroupedCardItem, boolean]
  /** 顺序变了：抛整树快照，由页面决定何时提交。 */
  orderChange: []
  groupCreate: []
  groupRename: [GroupedGroup]
  groupRemove: [GroupedGroup]
}>()

const label = computed(() => (props.kind === 'collection' ? '合集' : '项目'))

/** 拖动分组标题期间收起所有卡片：长列表下不收起就看不出拖到了哪。 */
const draggingGroup = ref(false)

/** 封面：上传的优先，没有就回退首篇；两者都没有时返回空串（渲染占位块，不发空 src）。 */
function coverOf(item: GroupedCardItem) {
  return resolveCollectionCover(item.cover, item.fallback_cover)?.url ?? ""
}

/** 任一拖拽结束都只通知"顺序变了"：快照由页面从当前数据现取。 */
function onDrop() {
  draggingGroup.value = false
  emit('orderChange')
}
</script>

<template>
  <div class="grouped-grid" :data-role="`${kind}-grouped-grid`">
    <p v-if="!props.groups.length && !props.ungrouped.length && !props.loading" class="admin-hint" :data-role="`${kind}-grid-empty`">
      还没有{{ label }}。点右上角「新建{{ label }}」开始；建好之后可以新建分组把它们归拢起来。
    </p>

    <!-- 未分组固定第一：它是"还没被放进任何分组"，不是一种分组。
         没有未分组条目时整段消失（空标题没有意义）。 -->
    <section v-if="props.ungrouped.length" class="grouped-grid__section" data-role="group-ungrouped">
      <header class="grouped-grid__head grouped-grid__head--static">
        <span class="grouped-grid__title">未分组</span>
        <span class="grouped-grid__count">{{ props.ungrouped.length }}</span>
      </header>

      <!-- 未分组也要跟着收起：拖动分组标题时如果它还铺着，
           屏幕上一半是卡片，"拖到哪儿了"又看不出来了。 -->
      <VueDraggable
        v-show="!draggingGroup"
        v-model="props.ungrouped"
        class="collection-grid"
        :animation="MOTION_EXPRESSIVE_MS.dragSettle"
        group="grouped-cards"
        ghost-class="collection-card--ghost"
        :data-role="`${kind}-ungrouped-cards`"
        @end="onDrop">
        <article
          v-for="item in props.ungrouped"
          :key="item.id"
          class="collection-card"
          :data-role="`${kind}-card`"
          :data-id="item.id">
          <div class="collection-card__cover">
            <img v-if="coverOf(item)" :src="coverOf(item)" :alt="item.title" />
            <span v-else class="collection-card__cover-empty" aria-hidden="true" />
          </div>
          <div class="collection-card__body">
            <strong class="collection-card__title">{{ item.title }}</strong>
            <p class="collection-card__desc">{{ item.description || '（还没有描述）' }}</p>
            <span class="admin-hint">{{ item.article_count ?? 0 }} 篇</span>
          </div>
          <footer class="collection-card__foot">
            <NSwitch
              :value="item.displayed"
              size="small"
              :loading="props.togglingId === item.id"
              :data-role="`${kind}-card-display`"
              @update:value="(v: boolean) => emit('toggle', item, v)" />
            <div class="collection-card__actions">
              <NButton size="tiny" tertiary :data-role="`${kind}-card-edit`" @click="emit('edit', item)">编辑</NButton>
              <NButton size="tiny" tertiary type="error" :data-role="`${kind}-card-delete`" @click="emit('remove', item)">
                删除
              </NButton>
            </div>
          </footer>
        </article>
      </VueDraggable>
    </section>

    <!-- 分组标题之间可拖动换序。 -->
    <VueDraggable
      v-model="props.groups"
      class="grouped-grid__groups"
      :animation="MOTION_EXPRESSIVE_MS.dragSettle"
      handle=".grouped-grid__grip"
      ghost-class="grouped-grid__section--ghost"
      :data-role="`${kind}-group-list`"
      @start="draggingGroup = true"
      @end="onDrop">
      <section
        v-for="group in props.groups"
        :key="group.id"
        class="grouped-grid__section"
        :data-role="`group-${group.id}`">
        <header class="grouped-grid__head">
          <span class="grouped-grid__grip" title="拖动调整分组顺序" aria-hidden="true">⠿</span>
          <span class="grouped-grid__title">{{ group.title }}</span>
          <span class="grouped-grid__count">{{ group.items.length }}</span>
          <div class="grouped-grid__actions">
            <NButton size="tiny" tertiary :data-role="`group-rename-${group.id}`" @click="emit('groupRename', group)">
              改名
            </NButton>
            <NButton
              size="tiny"
              tertiary
              type="error"
              :data-role="`group-delete-${group.id}`"
              @click="emit('groupRemove', group)">
              删除分组
            </NButton>
          </div>
        </header>

        <!-- 拖动分组标题时收起内容：只留一条标题，拖动目标一目了然。 -->
        <VueDraggable
          v-show="!draggingGroup"
          v-model="group.items"
          class="collection-grid"
          :animation="MOTION_EXPRESSIVE_MS.dragSettle"
          group="grouped-cards"
          ghost-class="collection-card--ghost"
          :data-role="`${kind}-group-cards-${group.id}`"
          @end="onDrop">
          <article
            v-for="item in group.items"
            :key="item.id"
            class="collection-card"
            :data-role="`${kind}-card`"
            :data-id="item.id">
            <div class="collection-card__cover">
              <img v-if="coverOf(item)" :src="coverOf(item)" :alt="item.title" />
              <span v-else class="collection-card__cover-empty" aria-hidden="true" />
            </div>
            <div class="collection-card__body">
              <strong class="collection-card__title">{{ item.title }}</strong>
              <p class="collection-card__desc">{{ item.description || '（还没有描述）' }}</p>
              <span class="admin-hint">{{ item.article_count ?? 0 }} 篇</span>
            </div>
            <footer class="collection-card__foot">
              <NSwitch
                :value="item.displayed"
                size="small"
                :loading="props.togglingId === item.id"
                :data-role="`${kind}-card-display`"
                @update:value="(v: boolean) => emit('toggle', item, v)" />
              <div class="collection-card__actions">
                <NButton size="tiny" tertiary :data-role="`${kind}-card-edit`" @click="emit('edit', item)">编辑</NButton>
                <NButton size="tiny" tertiary type="error" :data-role="`${kind}-card-delete`" @click="emit('remove', item)">
                  删除
                </NButton>
              </div>
            </footer>
          </article>
        </VueDraggable>

        <p v-if="!group.items.length && !draggingGroup" class="admin-hint grouped-grid__empty" :data-role="`group-empty-${group.id}`">
          把{{ label }}拖到这条标题上，就能放进这个分组。
        </p>
      </section>
    </VueDraggable>

    <div class="grouped-grid__new">
      <NButton size="small" :data-role="`${kind}-group-create`" @click="emit('groupCreate')">新建分组</NButton>
      <span v-if="props.savingOrder" class="admin-hint" data-role="group-order-saving">正在保存顺序…</span>
    </div>
  </div>
</template>
