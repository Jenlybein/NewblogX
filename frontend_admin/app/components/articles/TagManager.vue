<script setup lang="ts">
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
import { computed, h, onMounted, ref } from 'vue'
import {
  NButton,
  NCard,
  NDataTable,
  NInput,
  NInputNumber,
  NModal,
  NSpace,
  NSwitch,
  NTag,
  useDialog,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { deleteAdminTags, getAdminTags, saveAdminTag } from '~/services/admin'
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

const editorOpen = ref(false)
const editing = ref<TagListItem | null>(null)
const form = ref({ title: '', description: '', sort: 0, enabled: true })

async function refresh() {
  loading.value = true
  try {
    const data = await getAdminTags()
    rows.value = data.list || []
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

const columns: DataTableColumns<TagListItem> = [
  {
    title: '标签',
    key: 'title',
    minWidth: 220,
    render: row =>
      h('div', { class: 'min-w-0', 'data-role': 'tag-title' }, [
        h('strong', { class: 'block truncate' }, row.title || '（无标题）'),
        row.description
          ? h('span', { class: 'muted text-xs line-clamp-2' }, row.description)
          : null,
      ]),
  },
  {
    title: '文章数',
    key: 'article_count',
    width: 100,
    render: row => h('span', { 'data-role': 'tag-article-count' }, `${row.article_count ?? 0} 篇`),
  },
  {
    title: '排序',
    key: 'sort',
    width: 90,
    render: row => h('span', { 'data-role': 'tag-sort' }, String(row.sort ?? 0)),
  },
  {
    title: '状态',
    key: 'is_enabled',
    width: 100,
    render: row =>
      h(
        NTag,
        { size: 'small', type: row.is_enabled === false ? 'default' : 'success', 'data-role': 'tag-status' },
        { default: () => (row.is_enabled === false ? '停用' : '启用') },
      ),
  },
  {
    title: '操作',
    key: 'actions',
    width: 240,
    render: row =>
      h(NSpace, { size: 8 }, {
        default: () => [
          h(
            NButton,
            { size: 'small', tertiary: true, 'data-role': 'tag-edit', onClick: () => openEdit(row) },
            { default: () => '编辑' },
          ),
          h(
            NButton,
            {
              size: 'small',
              tertiary: true,
              type: row.is_enabled === false ? 'primary' : 'warning',
              loading: togglingId.value === row.id,
              'data-role': row.is_enabled === false ? 'tag-enable' : 'tag-disable',
              onClick: () => void toggleEnabled(row, row.is_enabled === false),
            },
            { default: () => (row.is_enabled === false ? '启用' : '停用') },
          ),
          h(
            NButton,
            { size: 'small', tertiary: true, type: 'error', 'data-role': 'tag-delete', onClick: () => remove(row) },
            { default: () => '删除' },
          ),
        ],
      }),
  },
]

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

    <NDataTable
      :columns="columns"
      :data="filteredRows"
      :loading="loading"
      :row-key="(row: TagListItem) => row.id"
      :bordered="false" />
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
      <NInputNumber v-model:value="form.sort" class="w-full" placeholder="排序（越大越靠前）" data-role="tag-editor-sort" />
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
