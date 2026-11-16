<script setup lang="ts">
/**
 * 站点 → 内容过滤（`/site/content-filter`，ADR-0032）。
 *
 * 这一页管第 11 段运行时配置 `content_filter`：全站唯一的词表 + 白名单。
 * 三件事必须一起说清楚，否则运营会按旧的两份词表习惯乱填：
 *
 *   1. **两档都替换**：拦截词与观察词命中后都变成等长 `*`，颜色只用于标识轻重；
 *   2. **匹配前会归一化**：全角、零宽字符、插空格与分隔符都拦得住，所以词条本身
 *      按正常写法填即可，不需要把变体逐个加进去；
 *   3. **保存即生效**：后端写完缓存就按新词表重建过滤引擎，不需要重启。
 *
 * 词云：字号 = 近 90 天命中次数、颜色 = 档位。命中统计的接口在后续票里接（风险页那批），
 * 本轮所有词条的次数都是 0，于是字号统一 —— 布局与颜色先按最终形状做出来，
 * 不造假数据。
 */
import {
  NAlert,
  NButton,
  NCard,
  NDynamicTags,
  NFormItem,
  NInput,
  NModal,
  NSelect,
  NSpace,
  NTabPane,
  NTabs,
  NTag,
  useMessage,
} from 'naive-ui'
import { getContentFilterRuntimeConfig, updateContentFilterRuntimeConfig } from '~/services/admin'
import type { ContentFilterRuntimeConfig, ContentFilterWord } from '~/types/api'

type Tier = ContentFilterWord['tier']
type Category = ContentFilterWord['category']

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
const loaded = ref(false)
// 初值只是占位：表单在 loaded 之前不渲染。
const config = ref({} as ContentFilterRuntimeConfig)

const activeTab = ref('words')
const keyword = ref('')
const categoryFilter = ref('all')
const hitCounts = ref<Record<string, number>>({})

const TIER_OPTIONS: { label: string, value: Tier }[] = [
  { label: '拦截词（明确违法/高危）', value: 'block' },
  { label: '观察词（推广话术为主）', value: 'watch' },
]
const CATEGORY_OPTIONS: { label: string, value: Category }[] = [
  { label: '毒品', value: 'drugs' },
  { label: '赌博', value: 'gambling' },
  { label: '诈骗', value: 'fraud' },
  { label: '色情', value: 'porn' },
  { label: '假证发票', value: 'fake_docs' },
  { label: '涉政', value: 'politics' },
  { label: '暴恐', value: 'violence' },
]
const TIER_LABEL: Record<string, string> = { block: '拦截词', watch: '观察词' }
const CATEGORY_LABEL: Record<string, string> = Object.fromEntries(
  CATEGORY_OPTIONS.map(option => [option.value, option.label]),
)

const words = computed<ContentFilterWord[]>(() => config.value.words ?? [])
const whitelist = computed<string[]>(() => config.value.whitelist ?? [])

/** 近 90 天命中次数（本轮无数据源，先全部按 0）。 */
function hitCountOf(word: string): number {
  return hitCounts.value[word] ?? 0
}

const maxHit = computed(() => words.value.reduce((max, entry) => Math.max(max, hitCountOf(entry.word)), 0))

/**
 * 字号 = 命中次数。次数全为 0 时统一 14px（不伪造差异）；
 * 有数据后按最热词线性映射到 14–40px，保证最热词不会被压成一行小字。
 */
function fontSizeOf(word: string): string {
  const count = hitCountOf(word)
  if (maxHit.value <= 0 || count <= 0) return '14px'
  return `${14 + Math.round((26 * count) / maxHit.value)}px`
}

const filteredWords = computed(() => {
  const needle = keyword.value.trim().toLowerCase()
  return words.value.filter((entry) => {
    if (categoryFilter.value !== 'all' && entry.category !== categoryFilter.value) return false
    if (!needle) return true
    return entry.word.toLowerCase().includes(needle)
  })
})

const blockCount = computed(() => words.value.filter(entry => entry.tier === 'block').length)
const watchCount = computed(() => words.value.filter(entry => entry.tier === 'watch').length)

async function load() {
  loading.value = true
  try {
    const current = await getContentFilterRuntimeConfig()
    // 后端把 nil 切片序列化成 null（老库的段可能两个字段都不全）：
    // 先在入口处补成空数组，模板里就不必到处判空。
    config.value = {
      words: current.words ?? [],
      whitelist: current.whitelist ?? [],
    }
    loaded.value = true
  } catch {
    message.error('内容过滤配置加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await updateContentFilterRuntimeConfig({
      words: words.value,
      whitelist: whitelist.value,
    })
    message.success('词表已保存；过滤器按新词表重建，立即生效')
  } catch {
    message.error('保存失败')
  } finally {
    saving.value = false
  }
}

// ── 点词编辑 ────────────────────────────────────────────────
// 编辑态用 string 承载下拉值：naive-ui 的 v-model 给的是 string，
// 直接绑到契约里窄化成联合类型的字段上会让 vue-tsc 报错。
const editing = ref<ContentFilterWord | null>(null)
// 打开编辑时的原词：编辑框里的文字会被改，定位/删除都要用改之前的那一个。
const editingKey = ref('')
const editTier = ref('watch')
const editCategory = ref('fraud')

function openEditor(entry: ContentFilterWord) {
  editing.value = { ...entry }
  editingKey.value = entry.word
  editTier.value = entry.tier
  editCategory.value = entry.category
}

function closeEditor() {
  editing.value = null
}

function applyEdit() {
  const entry = editing.value
  if (!entry) return
  const word = entry.word.trim()
  if (!word) {
    message.warning('词条不能为空')
    return
  }
  if (!isTier(editTier.value) || !isCategory(editCategory.value)) {
    message.warning('档位或类目不合规')
    return
  }
  const duplicated = words.value.some(existing => existing.word !== editingKey.value
    && existing.word.toLowerCase() === word.toLowerCase())
  if (duplicated) {
    message.warning(`词条「${word}」已经在表里了`)
    return
  }
  const index = words.value.findIndex(existing => existing.word === editingKey.value)
  if (index < 0) return
  words.value[index] = { word, tier: editTier.value, category: editCategory.value }
  closeEditor()
}

function removeEditing() {
  const entry = editing.value
  if (!entry) return
  config.value.words = words.value.filter(existing => existing.word !== editingKey.value)
  closeEditor()
}

// ── 批量导入（每行一词）────────────────────────────────────
const importing = ref(false)
const importText = ref('')
const importTier = ref('watch')
const importCategory = ref('fraud')

function openImport() {
  importText.value = ''
  importing.value = true
}

function applyImport() {
  if (!isTier(importTier.value) || !isCategory(importCategory.value)) {
    message.warning('档位或类目不合规')
    return
  }
  const existing = new Set(words.value.map(entry => entry.word.toLowerCase()))
  const added: ContentFilterWord[] = []
  let skipped = 0
  for (const line of importText.value.split(/\r?\n/u)) {
    const word = line.trim()
    if (!word) continue
    if (runeLength(word) < 2) {
      skipped += 1
      continue
    }
    const key = word.toLowerCase()
    if (existing.has(key)) {
      skipped += 1
      continue
    }
    existing.add(key)
    added.push({ word, tier: importTier.value, category: importCategory.value })
  }
  config.value.words = [...words.value, ...added]
  importing.value = false
  message.success(`已导入 ${added.length} 条，跳过 ${skipped} 条（重复或单字）`)
}

function isTier(value: string): value is Tier {
  return value === 'block' || value === 'watch'
}

function isCategory(value: string): value is Category {
  return CATEGORY_OPTIONS.some(option => option.value === value)
}

/** 位图里一个汉字算 1："词条太短"这条判据按字符数而不是字节数。 */
function runeLength(value: string): number {
  return Array.from(value).length
}

function onEditorShow(show: boolean) {
  if (!show) closeEditor()
}

function onImportShow(show: boolean) {
  importing.value = show
}

const categoryFilterOptions = computed(() => [
  { label: '全部类目', value: 'all' },
  ...CATEGORY_OPTIONS.map(option => ({ label: option.label, value: option.value })),
])

onMounted(load)
</script>

<template>
  <div class="admin-settings-panel">
    <NAlert type="info" :bordered="false" class="mb-4">
      这是全站<strong>唯一</strong>的词表：文章、评论、昵称、简介、私信、合集标题与描述都用它。
      拦截词与观察词命中后<strong>都替换成等长的 <code>*</code></strong>（颜色只用来分轻重），
      文本照常发布；命中明细进「审核 → 关键词风险」。
    </NAlert>
    <NAlert type="warning" :bordered="false" class="mb-4">
      匹配前会做归一化：全角折半角、拉丁转小写、零宽字符当分隔符、空白与标点被压掉。
      所以<strong>不用为变体逐个加词</strong>，按正常写法填一次就够；反过来，两个字以上的短词
      更容易误伤，填之前先想想它会不会出现在正常讨论里（单字词不入表）。
    </NAlert>
    <NAlert type="info" :bordered="false" class="mb-4">
      词表<strong>不会出现在任何匿名接口</strong>里，只有后台能读写。保存后立即生效，不需要重启。
    </NAlert>

    <NTabs v-if="loaded" v-model:value="activeTab" type="line">
      <NTabPane name="words" :tab="`词表（${words.length}）`">
        <NCard class="admin-card mb-4" size="small">
          <div class="admin-toolbar mb-4">
            <NSpace :size="8" align="center">
              <NInput v-model:value="keyword" placeholder="搜索词条" clearable class="admin-field--select" />
              <NSelect
                v-model:value="categoryFilter"
                :options="categoryFilterOptions"
                class="admin-field--select" />
            </NSpace>
            <NSpace :size="8" align="center">
              <NTag size="small" :bordered="false">拦截 {{ blockCount }}</NTag>
              <NTag size="small" :bordered="false">观察 {{ watchCount }}</NTag>
              <NButton size="small" data-role="content-filter-import" @click="openImport">批量导入</NButton>
              <NButton size="small" type="primary" :loading="saving" data-role="content-filter-save" @click="save">
                保存词表
              </NButton>
            </NSpace>
          </div>

          <NAlert type="info" :bordered="false" class="mb-4">
            字号 = 近 90 天命中次数，颜色 = 档位。命中统计随风险页一起接入，本轮尚未有数据，
            所以字号暂时统一；点任意一个词可以改档位、改类目或删除，改动先落在本页，点「保存词表」才写进后端。
          </NAlert>

          <div v-if="filteredWords.length > 0" class="cf-cloud" data-role="content-filter-cloud">
            <button
              v-for="entry in filteredWords"
              :key="entry.word"
              type="button"
              class="cf-word"
              :class="entry.tier === 'block' ? 'cf-word--block' : 'cf-word--watch'"
              :style="{ fontSize: fontSizeOf(entry.word) }"
              :title="`${TIER_LABEL[entry.tier]} · ${CATEGORY_LABEL[entry.category]} · 近 90 天命中 ${hitCountOf(entry.word)} 次`"
              @click="openEditor(entry)">
              {{ entry.word }}
            </button>
          </div>
          <div v-else class="admin-empty">
            {{ words.length === 0 ? '词表还是空的：先点「批量导入」贴入词条' : '没有匹配的词条' }}
          </div>
        </NCard>
      </NTabPane>

      <NTabPane name="whitelist" :tab="`白名单（${whitelist.length}）`">
        <NCard class="admin-card mb-4" size="small" title="白名单">
          <NAlert type="info" :bordered="false" class="mb-4">
            白名单是<strong>词或短语</strong>：被白名单短语完整包住的那次命中<strong>不替换</strong>，
            但仍会记一条命中（用于统计误伤）。例如把「冰毒科普」加进来，正文里的
            「冰毒科普」不会被替换，别处的「冰毒」照常替换。
          </NAlert>
          <NFormItem label="白名单词条（回车添加）">
            <NDynamicTags v-model:value="config.whitelist" data-role="content-filter-whitelist" />
          </NFormItem>
          <NButton type="primary" :loading="saving" @click="save">保存白名单</NButton>
        </NCard>
      </NTabPane>
    </NTabs>
    <div v-else class="admin-empty">{{ loading ? '配置加载中' : '配置加载失败，刷新页面重试' }}</div>

    <NModal
      :show="editing !== null"
      preset="card"
      style="width: 420px"
      title="编辑词条"
      @update:show="onEditorShow">
      <template v-if="editing">
        <NFormItem label="词条">
          <NInput v-model:value="editing.word" class="admin-field--key" />
        </NFormItem>
        <NFormItem label="档位">
          <NSelect v-model:value="editTier" :options="TIER_OPTIONS" class="admin-field--select" />
        </NFormItem>
        <NFormItem label="类目">
          <NSelect v-model:value="editCategory" :options="CATEGORY_OPTIONS" class="admin-field--select" />
        </NFormItem>
      </template>
      <NSpace justify="space-between">
        <NButton type="error" quaternary @click="removeEditing">删除</NButton>
        <NSpace :size="8">
          <NButton @click="closeEditor">取消</NButton>
          <NButton type="primary" @click="applyEdit">确定</NButton>
        </NSpace>
      </NSpace>
    </NModal>

    <NModal
      :show="importing"
      preset="card"
      style="width: 560px"
      title="批量导入词条"
      @update:show="onImportShow">
      <NAlert type="info" :bordered="false" class="mb-4">
        一行一个词，空行忽略；单字词与重复词会被跳过。导入只加到本页的待保存列表，
        点「保存词表」才会写进后端。
      </NAlert>
      <NFormItem label="导入后的默认档位">
        <NSelect v-model:value="importTier" :options="TIER_OPTIONS" class="admin-field--select" />
      </NFormItem>
      <NFormItem label="导入后的默认类目">
        <NSelect v-model:value="importCategory" :options="CATEGORY_OPTIONS" class="admin-field--select" />
      </NFormItem>
      <NFormItem label="词条（每行一词）">
        <NInput
          v-model:value="importText"
          type="textarea"
          :autosize="{ minRows: 8, maxRows: 16 }"
          class="admin-field--long"
          placeholder="代开发票&#10;办假证&#10;加微信" />
      </NFormItem>
      <NSpace justify="end" :size="8">
        <NButton @click="importing = false">取消</NButton>
        <NButton type="primary" @click="applyImport">导入</NButton>
      </NSpace>
    </NModal>
  </div>
</template>

<style scoped>
.cf-cloud {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  min-height: 120px;
  padding: 8px 4px;
  line-height: 1.5;
}

.cf-word {
  padding: 2px 10px;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-pill);
  background: var(--color-surface-raised);
  color: var(--color-ink);
  cursor: pointer;
  transition: transform var(--motion-fast) var(--motion-ease);
}

.cf-word:hover {
  transform: translateY(-1px);
  border-color: var(--color-line-strong);
}

.cf-word--block {
  color: var(--color-danger-ink);
  background: var(--color-danger-soft);
  border-color: var(--color-danger-line);
}

.cf-word--watch {
  color: var(--color-ink);
  background: var(--color-highlight);
}
</style>
