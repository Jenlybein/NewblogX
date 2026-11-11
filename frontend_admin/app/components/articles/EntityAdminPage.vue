<script setup lang="ts">
/**
 * 合集 / 项目管理页（票 348 起两页共用这一份实现）。
 *
 * 页面职责只剩"接线"：把卡片列表与分组树读进来、把拖拽后的顺序交给
 * useEntityGroups 的防抖保存、把分组 CRUD 立刻提交、把编辑弹窗打开。
 * 网格本身（分组标题 + 卡片 + 拖拽）在 GroupedEntityGrid，两页共用。
 *
 * 为什么两页共用一份实现：合集与项目的分组语义完全一致（ADR-0041），
 * 复制一份的话"删分组要把条目退回未分组"这类差异只会修在一边。
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { NButton, NInput, NModal, NSpace, useDialog, useMessage } from 'naive-ui'
import GroupedEntityGrid, { type GroupedCardItem, type GroupedGroup } from '~/components/articles/GroupedEntityGrid.vue'
import CollectionEditorOverlay from '~/components/articles/CollectionEditorOverlay.vue'
import { useEntityGroups } from '~/composables/useEntityGroups'
import {
  deleteAdminGroup,
  deleteCollection,
  deleteProject,
  getAdminCollections,
  getAdminGroups,
  getAdminProjects,
  replaceAdminGroupOrder,
  saveAdminGroup,
  setCollectionDisplay,
  setProjectDisplay,
} from '~/services/admin'

const props = withDefaults(defineProps<{
  kind: 'collection' | 'project'
}>(), {
  kind: 'collection',
})

const message = useMessage()
const dialog = useDialog()

const label = computed(() => (props.kind === 'collection' ? '合集' : '项目'))

const editorOpen = ref(false)
const editing = ref<GroupedCardItem | null>(null)
const togglingId = ref('')

const groups = useEntityGroups({
  kind: props.kind,
  loadCards: async () => {
    const data = props.kind === 'collection' ? await getAdminCollections() : await getAdminProjects()
    return (data.list || []) as unknown as GroupedCardItem[]
  },
  loadGroups: () => getAdminGroups(props.kind),
  saveOrder: async payload => {
    await replaceAdminGroupOrder(props.kind, payload)
  },
  onError: text => message.error(text),
})

// ── 分组弹窗（新建 / 改名共用一个，与收藏夹同一做法）──────────────

const groupEditorOpen = ref(false)
const editingGroupId = ref('')
const groupTitle = ref('')
const groupSaving = ref(false)

const groupTitleError = computed(() =>
  groupTitle.value.trim() ? '' : '请填写分组名称')

function openGroupCreate() {
  editingGroupId.value = ''
  groupTitle.value = ''
  groupEditorOpen.value = true
}

function openGroupRename(group: GroupedGroup) {
  editingGroupId.value = group.id
  groupTitle.value = group.title
  groupEditorOpen.value = true
}

async function submitGroup() {
  if (groupTitleError.value) return
  groupSaving.value = true
  try {
    await saveAdminGroup(props.kind, {
      ...(editingGroupId.value ? { id: editingGroupId.value } : {}),
      title: groupTitle.value.trim(),
    })
    groupEditorOpen.value = false
    message.success(editingGroupId.value ? '分组已改名' : '分组已新建')
    // 分组 CRUD 立即生效：马上重新归位，不等拖拽的 10 秒防抖。
    await groups.reloadAfterGroupChange()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '分组保存失败')
  } finally {
    groupSaving.value = false
  }
}

function removeGroup(group: GroupedGroup) {
  dialog.warning({
    title: '删除分组',
    content: `确认删除分组「${group.title}」？组里的${label.value}不会被删除，会退回到「未分组」。`,
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      try {
        await deleteAdminGroup(props.kind, group.id)
        message.success('分组已删除，组内内容退回未分组')
        await groups.reloadAfterGroupChange()
      } catch (error) {
        message.error(error instanceof Error ? error.message : '删除分组失败')
      }
    },
  })
}

// ── 卡片动作 ─────────────────────────────────────────────────

function openCreate() {
  editing.value = null
  editorOpen.value = true
}

function startEdit(item: GroupedCardItem) {
  editing.value = item
  editorOpen.value = true
}

async function handleSaved() {
  await groups.refresh()
}

async function toggleDisplay(item: GroupedCardItem, displayed: boolean) {
  if (togglingId.value) return
  togglingId.value = item.id
  try {
    if (props.kind === 'collection') await setCollectionDisplay(item.id, displayed)
    else await setProjectDisplay(item.id, displayed)
    item.displayed = displayed
    message.success(displayed ? `${label.value}已展示给读者` : `${label.value}已隐藏，读者侧不再可见`)
  } catch (error) {
    message.error(error instanceof Error ? `展示开关保存失败：${error.message}` : '展示开关保存失败，请稍后重试')
  } finally {
    togglingId.value = ''
  }
}

function removeItem(item: GroupedCardItem) {
  dialog.warning({
    title: `删除${label.value}`,
    content: `确认删除「${item.title || `这个${label.value}`}」？里面的文章不会被删除，但读者侧会立即看不到它。`,
    positiveText: '删除',
    negativeText: '取消',
    async onPositiveClick() {
      try {
        if (props.kind === 'collection') await deleteCollection(item.id)
        else await deleteProject(item.id)
        if (editing.value?.id === item.id) {
          editing.value = null
          editorOpen.value = false
        }
        message.success(`${label.value}已删除`)
        await groups.refresh()
      } catch (error) {
        message.error(error instanceof Error ? `删除失败：${error.message}` : '删除失败，请稍后重试')
      }
    },
  })
}

onMounted(groups.refresh)

// 离开页面前把待提交的顺序发出去：不等 10 秒防抖（否则切页就丢一次拖拽）。
onBeforeUnmount(() => {
  void groups.flushOrder()
})
</script>

<template>
  <NSpace vertical :size="16">
    <div class="admin-page__head">
      <div>
        <h1 class="admin-page__title">{{ label }}管理</h1>
        <p class="admin-hint">
          拖动卡片可以调顺序、也可以拖进别的分组；拖动分组标题（⠿）调整分组顺序，拖动期间内容会临时收起。
          顺序在停止拖动 10 秒后自动保存，离开页面前会立刻保存。分组的新建 / 改名 / 删除立即生效。
        </p>
      </div>
      <NButton type="primary" :data-role="`${kind}-create`" @click="openCreate">新建{{ label }}</NButton>
    </div>

    <GroupedEntityGrid
      :kind="kind"
      :groups="groups.groups.value"
      :ungrouped="groups.ungrouped.value"
      :loading="groups.loading.value"
      :toggling-id="togglingId"
      :saving-order="groups.sortState.value === 'saving'"
      @order-change="groups.markOrderDirty"
      @edit="startEdit"
      @remove="removeItem"
      @toggle="toggleDisplay"
      @group-create="openGroupCreate"
      @group-rename="openGroupRename"
      @group-remove="removeGroup" />

    <CollectionEditorOverlay
      :show="editorOpen"
      :kind="kind"
      :entity="editing"
      @close="editorOpen = false"
      @saved="handleSaved" />

    <!-- 分组弹窗：无标题条、按钮是「取消 / 确定」，与收藏夹资料弹窗同一形态。 -->
    <NModal
      v-model:show="groupEditorOpen"
      preset="card"
      class="admin-modal"
      :bordered="false"
      style="width: min(480px, calc(100vw - 32px))"
      :mask-closable="false">
      <label class="space-y-2">
        <span class="text-sm font-medium">分组名称</span>
        <NInput
          v-model:value="groupTitle"
          :maxlength="128"
          show-count
          placeholder="例如：2024 精选"
          :data-role="`${kind}-group-title`" />
        <span v-if="groupTitleError" class="admin-hint">{{ groupTitleError }}</span>
      </label>

      <template #footer>
        <NSpace justify="end">
          <NButton :data-role="`${kind}-group-cancel`" @click="groupEditorOpen = false">取消</NButton>
          <NButton
            type="primary"
            :loading="groupSaving"
            :disabled="Boolean(groupTitleError)"
            :data-role="`${kind}-group-submit`"
            @click="submitGroup">
            确定
          </NButton>
        </NSpace>
      </template>
    </NModal>
  </NSpace>
</template>
