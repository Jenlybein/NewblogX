<script setup lang="ts">
import { MOTION_EXPRESSIVE_MS } from '~/utils/motion'
/**
 * 标签管理（票 322）：全站词库唯一的人工写入口。
 *
 * 三条语义必须同时到位，少一条这页就会变成新的坑：
 *  1. **删除只对零引用标签开放**。被文章引用时后端拒绝并给出可读文案
 *     （"该标签已有 N 篇文章在用，请改用停用"），这里原样展示，不自己编一句
 *     "删除失败"把它盖掉；删除失败后列表不刷新（标签还在）。
 *  2. **编辑是读—改—写**。弹窗打开时把四栏的当前值全部填好、保存时全部带回，
 *     所以"只改标题"在线上表现为只变了标题；编辑一个停用标签后它仍是停用
 *     （提交的 is_enabled 是 false，后端对省略的字段也保持原值）。
 *  3. **标题重复给可读文案**。后端把唯一键冲突翻成「名称重复」，这里同样原样展示。
 *
 * 列表里的文章数是后端按关联表实时统计的，不是库里的冗余计数列；
 * 所以它和"能不能删"永远一致。
 */
import { computed, onMounted, ref, watch } from 'vue'
import {
  NButton,
  NCard,
  NInput,
  NModal,
  NSpace,
  NSwitch,
  useDialog,
  useMessage,
} from 'naive-ui'
import { VueDraggable } from 'vue-draggable-plus'
import TagCard from '~/components/articles/TagCard.vue'
import { deleteAdminTags, getAdminTags, replaceAdminTagOrder, saveAdminTag } from '~/services/admin'
import { useSortPersist } from '~/composables/useSortPersist'
import type { TagListItem, TagWritePayload } from '~/types/api'

const message = useMessage()
const dialog = useDialog()

const loading = ref(false)
const saving = ref(false)
const togglingId = ref('')
const rows = ref<TagListItem[]>([])
const keyword = ref('')

/** 按标题过滤是纯前端行为：标签是全量返回的词库，不需要再走一次接口。 */
const filteredRows = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return rows.value
  return rows.value.filter(row => (row.title || '').toLowerCase().includes(kw))
})

/**
 * 拖拽排序只在"不过滤"时允许。
 *
 * 过滤状态下可见的是全集的一个子集，把子集的顺序当整份顺序提交会把被隐藏的
 * 标签挤到最后 —— 用户看不出为什么刷新后顺序变了。所以过滤时禁用拖拽，
 * 并在提示里说清楚。
 */
const dragEnabled = computed(() => keyword.value.trim() === '')

/**
 * 上一次**提交成功**的顺序快照：提交失败时回滚到它。
 *
 * 只存 id 数组而不是整行，回滚时按 id 重排当前行 —— 期间若有人改名/启停，
 * 那些字段变更不该被这次回滚覆盖。
 *
 * 快照只在两个时刻更新：从服务端读回列表（这就是库里的真实顺序）、以及一次
 * 提交真的成功之后。拖动时更新快照会让"上一次成功"变成"上一次界面"，
 * 失败回滚就回不到库里的状态了。
 */
let lastSavedOrder: string[] = []

function snapshotOrder() {
  lastSavedOrder = rows.value.map(row => row.id)
}

/** 提交时的顺序：只发全集（含停用），后端要求它是当前词库的一个排列。 */
const sortPersist = useSortPersist({
  commit: async () => {
    await replaceAdminTagOrder(rows.value.map(row => row.id))
    snapshotOrder()
  },
  onError: () => {
    message.error('排序保存失败，正在恢复上一次成功的顺序')
  },
})

/** 拖动结束：标记脏并触发 10 秒防抖；失败时按快照回滚。 */
function onOrderChange() {
  if (!dragEnabled.value) return
  sortPersist.markDirty()
}

watch(() => sortPersist.state.value, (state) => {
  if (state !== 'error' || lastSavedOrder.length === 0) return
  const byId = new Map(rows.value.map(row => [row.id, row]))
  const restored = lastSavedOrder
    .map(id => byId.get(id))
    .filter((row): row is TagListItem => Boolean(row))
  if (restored.length === rows.value.length) rows.value = restored
})

/** 保存状态文案：让用户知道"刚才那一下拖动有没有落地"。 */
const sortStateLabel = computed(() => {
  switch (sortPersist.state.value) {
    case 'pending': return '顺序待保存（停止拖动 10 秒后自动保存）'
    case 'saving': return '正在保存顺序…'
    case 'error': return '顺序保存失败，已恢复上一次成功的顺序'
    default: return '顺序已保存（拖动后 10 秒自动保存）'
  }
})

const editorOpen = ref(false)
const editing = ref<TagListItem | null>(null)
const form = ref({ title: '', description: '', sort: 0, enabled: true })

async function refresh() {
  loading.value = true
  try {
    const data = await getAdminTags()
    rows.value = data.list || []
    snapshotOrder()
  } catch {
    message.error('标签列表加载失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  form.value = { title: '', description: '', sort: 0, enabled: true }
  editorOpen.value = true
}

/**
 * 打开编辑：四栏都从当前行回填。
 *
 * 全部回填而不是"只带回用户改过的那几栏"，是因为"没传就保留原值"在界面上
 * 看不出来；把当前值显式提交，用户看到的表单就是他最后保存的形态。
 */
function openEdit(row: TagListItem) {
  editing.value = row
  form.value = {
    title: row.title || '',
    description: row.description || '',
    sort: row.sort ?? 0,
    enabled: row.is_enabled !== false,
  }
  editorOpen.value = true
}

function buildPayload(): TagWritePayload {
  const payload: TagWritePayload = {
    title: form.value.title.trim(),
    description: form.value.description,
    sort: form.value.sort,
    is_enabled: form.value.enabled,
  }
  if (editing.value?.id) payload.id = editing.value.id
  return payload
}

async function save() {
  if (!form.value.title.trim()) {
    message.warning('标签标题不能为空')
    return
  }
  saving.value = true
  try {
    await saveAdminTag(buildPayload())
    editorOpen.value = false
    message.success(editing.value ? '标签已更新' : '标签已新建')
    // 新建 / 改名 / 改排序都要在列表里看得见：以服务端为准重新拉一次。
    await refresh()
  } catch (error) {
    // 「名称重复」等可读文案由后端给出，原样展示，不暴露数据库错误。
    message.error(error instanceof Error ? error.message : '保存失败，请稍后重试')
  } finally {
    saving.value = false
  }
}

/**
 * 启停单个标签。
 *
 * 走同一个保存接口：带上这一行的其余字段，只把 is_enabled 翻过来。
 * 停用不影响已引用它的文章（不自动摘除），也不会删掉这个词。
 */
async function toggleEnabled(row: TagListItem, enabled: boolean) {
  if (togglingId.value) return
  togglingId.value = row.id
  try {
    await saveAdminTag({
      id: row.id,
      title: row.title,
      sort: row.sort,
      description: row.description,
      is_enabled: enabled,
    })
    row.is_enabled = enabled
    message.success(enabled ? '标签已启用，发布页下拉里重新可选' : '标签已停用，发布页下拉里不再出现')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '启停失败，请稍后重试')
  } finally {
    togglingId.value = ''
  }
}

function remove(row: TagListItem) {
  dialog.warning({
    title: '删除标签',
    content: `确认删除「${row.title}」？只有没被任何文章引用的标签才删得掉；文章还在用它的，请改用停用。`,
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      try {
        await deleteAdminTags([row.id])
        message.success('删除成功')
        await refresh()
      } catch (error) {
        // 被引用时后端返回带篇数的可读文案；原样给用户看，他才知道该改用停用。
        message.error(error instanceof Error ? error.message : '删除失败，请稍后重试')
      }
    },
  })
}

onMounted(refresh)
</script>

<template>
  <NCard class="admin-card" title="标签词库">
    <template #header-extra>
      <NButton type="primary" size="small" data-role="tag-create" @click="openCreate">新建标签</NButton>
    </template>

    <div class="admin-toolbar">
      <NInput
        v-model:value="keyword"
        class="w-full md:max-w-[360px]"
        placeholder="按标题过滤标签"
        clearable
        data-role="tag-filter" />
      <span class="muted text-xs">共 {{ rows.length }} 个标签（含停用），当前显示 {{ filteredRows.length }} 个</span>
    </div>

    <p class="admin-hint" data-role="tag-sort-hint">
      {{ dragEnabled ? `拖动卡片可调整标签顺序（发布页下拉与标签列表同序）· ${sortStateLabel}` : '过滤状态下不可拖动排序：先清空搜索框，避免把隐藏的标签挤到最后。' }}
    </p>

    <!-- 可拖动时是 VueDraggable（拖动把手由 draggable 打开），过滤时退化成普通
         网格 —— 两条分支渲染的是同一个 TagCard，不再各写一份。 -->
    <VueDraggable
      v-if="dragEnabled"
      v-model="rows"
      class="tag-grid"
      :animation="MOTION_EXPRESSIVE_MS.dragSettle"
      handle=".tag-card__grip"
      ghost-class="tag-card--ghost"
      data-role="tag-grid"
      @end="onOrderChange">
      <TagCard
        v-for="row in filteredRows"
        :key="row.id"
        :row="row"
        draggable
        :toggling="togglingId === row.id"
        @edit="openEdit(row)"
        @toggle="void toggleEnabled(row, row.is_enabled === false)"
        @remove="remove(row)" />
    </VueDraggable>

    <div v-else class="tag-grid" data-role="tag-grid">
      <TagCard
        v-for="row in filteredRows"
        :key="row.id"
        :row="row"
        :toggling="togglingId === row.id"
        @edit="openEdit(row)"
        @toggle="void toggleEnabled(row, row.is_enabled === false)"
        @remove="remove(row)" />
    </div>
    <p v-if="!loading && filteredRows.length === 0" class="admin-empty" data-role="tag-empty">
      没有匹配的标签。
    </p>
  </NCard>

  <NModal
    v-model:show="editorOpen"
    preset="card"
    class="admin-modal"
    :title="editing ? '编辑标签' : '新建标签'"
    style="width: 520px">
    <NSpace vertical :size="12">
      <NInput v-model:value="form.title" placeholder="标签标题（必填）" data-role="tag-editor-title" />
      <NInput
        v-model:value="form.description"
        type="textarea"
        :rows="2"
        placeholder="描述（可选，仅后台可见）"
        data-role="tag-editor-description" />
      <!-- 排序不再手填：唯一入口是网格里的拖拽，手填数字会变成第二套顺序真相。 -->
      <p class="admin-hint">
        排序在列表里拖动卡片调整（停止拖动 10 秒后自动保存），新建的标签排在最后。
      </p>
      <div class="flex items-center gap-3">
        <NSwitch v-model:value="form.enabled" data-role="tag-editor-enabled" />
        <span class="muted text-sm">启用（停用后不再出现在发布页的标签下拉里）</span>
      </div>
    </NSpace>

    <template #footer>
      <NSpace justify="end">
        <NButton data-role="tag-editor-cancel" @click="editorOpen = false">取消</NButton>
        <NButton type="primary" :loading="saving" data-role="tag-editor-save" @click="save">
          {{ editing ? '保存' : '创建' }}
        </NButton>
      </NSpace>
    </template>
  </NModal>
</template>
