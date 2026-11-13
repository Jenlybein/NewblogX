<script setup lang="ts">
import { MOTION_EXPRESSIVE_MS } from '~/utils/motion'
/**
 * 合集 / 项目分组网格（票 348，两个页面共用一份；2026-11-12 重排）。
 *
 * 结构（与 ADR-0019 的读面一一对应）：
 *
 *   ┌ ⠿ 未分组          3 个 ─────────────────────┐  ← 固定第一，空时整段消失
 *   │  [瓦片] [瓦片] [瓦片] … （最多 11 列）        │
 *   └──────────────────────────────────────────────┘
 *   ┌ ⠿ 第一组          5 个     改名  删除分组 ──┐  ← 标题栏本身就是拖拽把手
 *   │  [瓦片] [瓦片] …                              │
 *   └──────────────────────────────────────────────┘
 *
 * 三条拖拽语义：
 *  1. **瓦片拖进别的分组** = 改归属（跨组移动）；
 *  2. **瓦片在组内拖动** = 改组内顺序；
 *  3. **拖分组标题栏** = 改分组之间的顺序；拖动期间把各组内容临时收起，
 *     否则长列表下"拖到哪儿了"完全看不出来。
 *
 * 2026-11-12 的变化：卡片从"封面 + 标题 + 两行描述 + 脚栏"的大卡收成
 * **约 100px 宽的小瓦片**（11 列），描述退出卡面（编辑弹窗里仍有），
 * 分组标题从一行裸文字变成有底、有把手、有计数与操作的分组栏。
 *
 * 组件保持"哑"：不发请求、不持保存状态，顺序变化只抛 `orderChange`
 * （整树快照），由页面用 useSortPersist 做 10 秒防抖 + 离开前 flush。
 */
import { computed, ref, watch } from 'vue'
import { NButton, NSwitch } from 'naive-ui'
import { IconChevronDown, IconChevronRight, IconGripVertical, IconPencil, IconTrash } from '@tabler/icons-vue'
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
  /** 顺序变了：抛整树快照（拖拽后的真实排列），由页面决定何时提交。 */
  orderChange: [snapshot: GroupedTreeSnapshot]
  groupCreate: []
  groupRename: [GroupedGroup]
  groupRemove: [GroupedGroup]
}>()

const label = computed(() => (props.kind === 'collection' ? '合集' : '项目'))

/**
 * 整树快照：拖完之后界面上的真实排列。
 *
 * 组件**不直接改 props**（`v-model` 绑 props 会触发 vue/no-mutating-props，
 * 更实际的问题是：父组件的数据被子里悄悄改掉，谁在什么时候改的说不清）。
 * 所以内部持一份本地副本，拖拽只动副本，松手时把快照抛给父组件去落账。
 */
export interface GroupedTreeSnapshot {
  groups: { id: string; items: string[] }[]
  ungrouped: string[]
}

const localGroups = ref<GroupedGroup[]>([])
const localUngrouped = ref<GroupedCardItem[]>([])

// props → 本地副本：父组件刷新（重新归位）时同步过来。
watch(
  () => props.groups,
  (next) => {
    localGroups.value = next.map(group => ({ ...group, items: [...group.items] }))
  },
  { immediate: true, deep: true },
)
watch(
  () => props.ungrouped,
  (next) => {
    localUngrouped.value = [...next]
  },
  { immediate: true, deep: true },
)

/** 拖动分组标题期间收起所有卡片：长列表下不收起就看不出拖到了哪。 */
const draggingGroup = ref(false)

/**
 * 折叠分组（2026-11-12）。键是分组 id，未分组段用固定键。
 *
 * 与「拖动标题时临时收起」是两套状态：`draggingGroup` 是瞬时的，折叠是读者
 * 自己选的。两者任一成立就不渲染卡片，但只有折叠会在拖完之后保留。
 */
const UNGROUPED_KEY = '__ungrouped__'
const collapsed = ref<Record<string, boolean>>({})

function isCollapsed(key: string) {
  return Boolean(collapsed.value[key])
}

function toggleCollapsed(key: string) {
  collapsed.value = { ...collapsed.value, [key]: !collapsed.value[key] }
}

/** aria-controls 的目标：卡片列表的 id 与它同源。 */
function cardsPanelId(key: string) {
  return `${props.kind}-group-panel-${key}`
}

/** 封面：上传的优先，没有就回退首篇；两者都没有时返回空串（渲染占位块，不发空 src）。 */
function coverOf(item: GroupedCardItem) {
  return resolveCollectionCover(item.cover, item.fallback_cover)?.url ?? ""
}

/** 任一拖拽结束：收起结束，并把当前排列作为快照抛给父组件。 */
function onDrop() {
  draggingGroup.value = false
  emit('orderChange', {
    groups: localGroups.value.map(group => ({ id: group.id, items: group.items.map(item => item.id) })),
    ungrouped: localUngrouped.value.map(item => item.id),
  })
}
</script>

<template>
  <div class="grouped-grid" :class="{ 'grouped-grid--dragging': draggingGroup }" :data-role="`${kind}-grouped-grid`">
    <p v-if="!localGroups.length && !localUngrouped.length && !props.loading" class="admin-hint" :data-role="`${kind}-grid-empty`">
      还没有{{ label }}。点右上角「新建{{ label }}」开始；建好之后可以新建分组把它们归拢起来。
    </p>

    <!-- 未分组固定第一：它是"还没被放进任何分组"，不是一种分组。
         没有未分组条目时整段消失（空标题没有意义）。 -->
    <section v-if="localUngrouped.length" class="grouped-grid__section" data-role="group-ungrouped">
      <header class="grouped-grid__head grouped-grid__head--static">
        <button
          type="button"
          class="grouped-grid__collapse"
          :aria-expanded="!isCollapsed(UNGROUPED_KEY)"
          :aria-controls="cardsPanelId(UNGROUPED_KEY)"
          :aria-label="`${isCollapsed(UNGROUPED_KEY) ? '展开' : '收起'}未分组`"
          :data-role="`${kind}-group-collapse-ungrouped`"
          @mousedown.stop
          @click.stop="toggleCollapsed(UNGROUPED_KEY)">
          <IconChevronDown v-if="!isCollapsed(UNGROUPED_KEY)" :size="14" aria-hidden="true" />
          <IconChevronRight v-else :size="14" aria-hidden="true" />
        </button>
        <span class="grouped-grid__title">未分组</span>
        <span class="grouped-grid__count">{{ localUngrouped.length }} 个</span>
        <span class="grouped-grid__hint-inline">拖到下面任一标题栏即可归组</span>
      </header>

      <!-- 未分组也要跟着收起：拖动分组标题时如果它还铺着，
           屏幕上一半是卡片，"拖到哪儿了"又看不出来了。 -->
      <VueDraggable
        v-show="!draggingGroup && !isCollapsed(UNGROUPED_KEY)"
        :id="cardsPanelId(UNGROUPED_KEY)"
        v-model="localUngrouped"
        class="entity-tile-grid"
        :animation="MOTION_EXPRESSIVE_MS.dragSettle"
        group="grouped-cards"
        ghost-class="entity-tile--ghost"
        :data-role="`${kind}-ungrouped-cards`"
        @end="onDrop">
        <article
          v-for="item in localUngrouped"
          :key="item.id"
          class="entity-tile"
          :class="{ 'entity-tile--off': !item.displayed }"
          :data-role="`${kind}-card`"
          :data-id="item.id">
          <div class="entity-tile__cover">
            <img v-if="coverOf(item)" :src="coverOf(item)" :alt="item.title">
            <span v-else class="entity-tile__cover-empty" aria-hidden="true" />
            <div class="entity-tile__hover">
              <button
                type="button"
                class="entity-tile__action"
                title="编辑"
                aria-label="编辑"
                :data-role="`${kind}-card-edit`"
                @click="emit('edit', item)">
                <IconPencil :size="14" aria-hidden="true" />
              </button>
              <button
                type="button"
                class="entity-tile__action entity-tile__action--danger"
                title="删除"
                aria-label="删除"
                :data-role="`${kind}-card-delete`"
                @click="emit('remove', item)">
                <IconTrash :size="14" aria-hidden="true" />
              </button>
            </div>
            <span v-if="!item.displayed" class="entity-tile__badge">已隐藏</span>
          </div>
          <div class="entity-tile__body">
            <strong class="entity-tile__title" :title="item.title">{{ item.title }}</strong>
            <div class="entity-tile__foot">
              <span class="entity-tile__count">{{ item.article_count ?? 0 }} 篇</span>
              <NSwitch
                :value="item.displayed"
                size="small"
                :loading="props.togglingId === item.id"
                :aria-label="item.displayed ? '当前展示中，点击隐藏' : '当前已隐藏，点击展示'"
                :data-role="`${kind}-card-display`"
                @update:value="(v: boolean) => emit('toggle', item, v)" />
            </div>
          </div>
        </article>
      </VueDraggable>
    </section>

    <!-- 分组标题之间可拖动换序：整条标题栏都是把手（grip 只是视觉提示）。 -->
    <VueDraggable
      v-model="localGroups"
      class="grouped-grid__groups"
      :animation="MOTION_EXPRESSIVE_MS.dragSettle"
      handle=".grouped-grid__head"
      ghost-class="grouped-grid__section--ghost"
      :data-role="`${kind}-group-list`"
      @start="draggingGroup = true"
      @end="onDrop">
      <section
        v-for="group in localGroups"
        :key="group.id"
        class="grouped-grid__section"
        :data-role="`group-${group.id}`">
        <header class="grouped-grid__head" title="拖动调整分组顺序">
          <!-- 折叠按钮在拖拽把手之内：按下它不能起拖（mousedown.stop），
               点一下只切换折叠，不改变分组顺序。 -->
          <button
            type="button"
            class="grouped-grid__collapse"
            :aria-expanded="!isCollapsed(group.id)"
            :aria-controls="cardsPanelId(group.id)"
            :aria-label="`${isCollapsed(group.id) ? '展开' : '收起'}分组：${group.title}`"
            :data-role="`group-collapse-${group.id}`"
            @mousedown.stop
            @click.stop="toggleCollapsed(group.id)">
            <IconChevronDown v-if="!isCollapsed(group.id)" :size="14" aria-hidden="true" />
            <IconChevronRight v-else :size="14" aria-hidden="true" />
          </button>
          <span class="grouped-grid__grip" aria-hidden="true"><IconGripVertical :size="16" /></span>
          <h3 class="grouped-grid__title">{{ group.title }}</h3>
          <span class="grouped-grid__count">{{ group.items.length }} 个</span>
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
          v-show="!draggingGroup && !isCollapsed(group.id)"
          :id="cardsPanelId(group.id)"
          v-model="group.items"
          class="entity-tile-grid"
          :animation="MOTION_EXPRESSIVE_MS.dragSettle"
          group="grouped-cards"
          ghost-class="entity-tile--ghost"
          :data-role="`${kind}-group-cards-${group.id}`"
          @end="onDrop">
          <article
            v-for="item in group.items"
            :key="item.id"
            class="entity-tile"
            :class="{ 'entity-tile--off': !item.displayed }"
            :data-role="`${kind}-card`"
            :data-id="item.id">
            <div class="entity-tile__cover">
              <img v-if="coverOf(item)" :src="coverOf(item)" :alt="item.title">
              <span v-else class="entity-tile__cover-empty" aria-hidden="true" />
              <div class="entity-tile__hover">
                <button
                  type="button"
                  class="entity-tile__action"
                  title="编辑"
                  aria-label="编辑"
                  :data-role="`${kind}-card-edit`"
                  @click="emit('edit', item)">
                  <IconPencil :size="14" aria-hidden="true" />
                </button>
                <button
                  type="button"
                  class="entity-tile__action entity-tile__action--danger"
                  title="删除"
                  aria-label="删除"
                  :data-role="`${kind}-card-delete`"
                  @click="emit('remove', item)">
                  <IconTrash :size="14" aria-hidden="true" />
                </button>
              </div>
              <span v-if="!item.displayed" class="entity-tile__badge">已隐藏</span>
            </div>
            <div class="entity-tile__body">
              <strong class="entity-tile__title" :title="item.title">{{ item.title }}</strong>
              <div class="entity-tile__foot">
                <span class="entity-tile__count">{{ item.article_count ?? 0 }} 篇</span>
                <NSwitch
                  :value="item.displayed"
                  size="small"
                  :loading="props.togglingId === item.id"
                  :aria-label="item.displayed ? '当前展示中，点击隐藏' : '当前已隐藏，点击展示'"
                  :data-role="`${kind}-card-display`"
                  @update:value="(v: boolean) => emit('toggle', item, v)" />
              </div>
            </div>
          </article>
        </VueDraggable>

        <p v-if="!group.items.length && !draggingGroup && !isCollapsed(group.id)" class="admin-hint grouped-grid__empty" :data-role="`group-empty-${group.id}`">
          把{{ label }}拖到这条标题上，就能放进这个分组。
        </p>
      </section>
    </VueDraggable>

    <div class="grouped-grid__new">
      <NButton size="small" dashed :data-role="`${kind}-group-create`" @click="emit('groupCreate')">＋ 新建分组</NButton>
      <span v-if="props.savingOrder" class="admin-hint" data-role="group-order-saving">正在保存顺序…</span>
    </div>
  </div>
</template>
