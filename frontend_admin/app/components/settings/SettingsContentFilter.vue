<script setup lang="ts">
/**
 * 内容修改与发布 → 关键词过滤（`/site/content-filter`，ADR-0032）。
 *
 * 这一页管第 11 段运行时配置 `content_filter`：全站唯一的词表 + 白名单。
 * 三件事必须一起说清楚，否则运营会按旧的两份词表习惯乱填：
 *
 *   1. **两档都替换**：拦截词与观察词命中后都变成等长 `*`，颜色只用于标识轻重；
 *   2. **匹配前会归一化**：全角、零宽字符、插空格与分隔符都拦得住，所以词条本身
 *      按正常写法填即可，不需要把变体逐个加进去；
 *   3. **保存即生效**：后端写完缓存就按新词表重建过滤引擎，不需要重启。
 *
 * 词云：字号 = 近 90 天命中处数、颜色 = 档位。次数来自
 * `GET /content-filter/word-stats`（窗口由后端定，响应回显 days，前端不自算 90）。
 * 统计读不到时不影响本页：词表照常可编辑，字号退化成统一 14px（不伪造差异）。
 */
import {
  NAlert,
  NButton,
  NCard,
  NDynamicTags,
  NFormItem,
  NInput,
  NInputNumber,
  NModal,
  NSelect,
  NSpace,
  NTabPane,
  NTabs,
  NTag,
  useMessage,
} from 'naive-ui'
import {
  getContentFilterRuntimeConfig,
  getContentFilterWordStats,
  updateContentFilterRuntimeConfig,
} from '~/services/admin'
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
// 窗口天数由后端响应回显：页面上要显示"最近 N 天"，N 不该在这里写死。
const hitWindowDays = ref(90)

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

/**
 * 必需非空的类目（与后端 contentfilter.RequiredCategories 同一口径）。
 *
 * 清单由**后端下发**（content_filter 段响应里的 required_categories /
 * missing_required_categories），前端不再自己抄一份类目列表：抄一份就等于同一件事
 * 有了两个真相，改一处忘另一处时，红条会在一台机器上安静、在另一台上尖叫。
 */
const missingRequiredCategories = computed<string[]>(
  () => config.value.missing_required_categories ?? [],
)
const whitelist = computed<string[]>(() => config.value.whitelist ?? [])
const whitelistPhrases = computed<string[]>(() => config.value.whitelist_phrases ?? [])
// 计数用后端算好的口径（去重后才对得上 `-filter-coverage` 打印的数）；
// 前端自己把两个数组相加会得到"页面 50、体检 49"。
const whitelistTotal = computed(() => config.value.whitelist_stats?.total ?? 0)

/** 窗口内命中处数（来自 word-stats；没有命中记录时为 0）。 */
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

/**
 * 词云只渲染前 `renderLimit` 个。
 *
 * 词表已经有 6 万条：一次性把 6 万个 <button> 塞进 DOM，浏览器会卡死好几秒，
 * 而运营真正要看的是"搜索 + 按类目收窄后的那几十个"。所以默认只渲染一窗，
 * 底部给出"还有 N 个"并允许逐窗展开；搜索/类目变化时窗口回到第一页。
 */
const RENDER_WINDOW = 600
const renderLimit = ref(RENDER_WINDOW)
const visibleWords = computed(() => filteredWords.value.slice(0, renderLimit.value))
const hiddenWordCount = computed(() => Math.max(0, filteredWords.value.length - visibleWords.value.length))

function showMoreWords() {
  renderLimit.value += RENDER_WINDOW
}

// 收窄条件变了就把窗口收回第一页，否则"搜到 3 个但窗口还停在 6000"会让人以为没搜到。
watch([keyword, categoryFilter], () => {
  renderLimit.value = RENDER_WINDOW
})

const blockCount = computed(() => words.value.filter(entry => entry.tier === 'block').length)
const watchCount = computed(() => words.value.filter(entry => entry.tier === 'watch').length)

/**
 * 把后端返回的段落到表单上。
 *
 * 后端把 nil 切片序列化成 null（老库的段可能字段不全）：在入口处补成空数组，
 * 模板里就不必到处判空。required/missing 原样保留 —— 红条只认后端给的结论。
 */
function applyConfig(current: ContentFilterRuntimeConfig) {
  config.value = {
    ...current,
    words: current.words ?? [],
    whitelist: current.whitelist ?? [],
    whitelist_phrases: current.whitelist_phrases ?? [],
    retain_days: current.retain_days ?? 90,
  }
}

async function load() {
  loading.value = true
  try {
    applyConfig(await getContentFilterRuntimeConfig())
    loaded.value = true
  } catch {
    message.error('内容过滤配置加载失败')
  } finally {
    loading.value = false
  }
  // 词云统计单独拉：它失败不该让整页打不开（词表编辑才是本页的主功能）。
  await loadWordStats()
}

/** 拉一次词云统计；失败只记一次提示，不清空既有值。 */
async function loadWordStats() {
  try {
    const stats = await getContentFilterWordStats()
    hitWindowDays.value = stats.days || 90
    hitCounts.value = Object.fromEntries(stats.items.map(item => [item.word, item.count]))
  } catch {
    // 统计是辅助展示：静默退化成"全部 0 次"，不打断词表编辑。
  }
}

async function save() {
  saving.value = true
  try {
    await updateContentFilterRuntimeConfig({
      words: words.value,
      whitelist: whitelist.value,
      whitelist_phrases: whitelistPhrases.value,
      retain_days: config.value.retain_days,
    })
    // 保存后重新拉一次：缺口清单是后端算的，改完词表要按新词表重新判，
    // 否则刚删空某一类的红条要等下次进页面才出现。
    applyConfig(await getContentFilterRuntimeConfig())
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

    <!-- 类目覆盖自检（ADR-0032 决策 8）：
         涉政与暴恐两类按设计只进数据库、不进仓库，所以仓库与 CI 全绿并不代表线上
         词库完整。2026-11-22 生产实测就是这两类各 0 条，而当时六个文本面
         对涉政内容完全不拦——这个红条就是为了让那种情况没法再静默通过。

         必须等 loaded 再判断：加载中 config 是空对象，missing 会被算成"两类都缺"，
         红条就在每次进页面时先闪一下——那种"永远在报警"的红条等于没有红条。 -->
    <NAlert
      v-if="loaded && missingRequiredCategories.length > 0"
      type="error"
      :bordered="false"
      class="mb-4">
      <strong>缺少必需类目的词条：{{ missingRequiredCategories.map(c => CATEGORY_LABEL[c] ?? c).join('、') }}。</strong>
      这几类按设计<strong>只进数据库、不进仓库</strong>，必须由部署侧生成后导入（在后台逐条添加几万条并不现实）：
      在<strong>有 node 的机器</strong>上跑
      <code>BLOGX_VIOLENCE_HINTS=&lt;私有暴恐词文件&gt; node scripts/content-filter/build-wordlist.mjs --include-all --out words.tsv</code>，
      传到部署机后 <code>server -f config/settings.yaml -filter-import words.tsv -filter-import-apply</code>，再重启服务。
      详见 README「内容过滤：词表的生成、导入与体检」。导入前这些类目对应内容处于<strong>完全不过滤</strong>状态。
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
            字号 = 最近 {{ hitWindowDays }} 天的命中处数（来自后端统计），颜色 = 档位；没有命中记录的词统一小字号。
            词表已有 {{ words.length }} 条，页面一次只渲染一窗，用搜索 / 类目收窄最省事。
            点任意一个词可以改档位、改类目或删除，改动先落在本页，点「保存词表」才写进后端。
          </NAlert>

          <div v-if="visibleWords.length > 0" class="cf-cloud" data-role="content-filter-cloud">
            <button
              v-for="entry in visibleWords"
              :key="entry.word"
              type="button"
              class="cf-word"
              :class="entry.tier === 'block' ? 'cf-word--block' : 'cf-word--watch'"
              :style="{ fontSize: fontSizeOf(entry.word) }"
              :title="`${TIER_LABEL[entry.tier]} · ${CATEGORY_LABEL[entry.category]} · 最近 ${hitWindowDays} 天命中 ${hitCountOf(entry.word)} 处`"
              @click="openEditor(entry)">
              {{ entry.word }}
            </button>
          </div>
          <div v-else class="admin-empty">
            {{ words.length === 0 ? '词表还是空的：先点「批量导入」贴入词条' : '没有匹配的词条' }}
          </div>
          <div v-if="hiddenWordCount > 0" class="admin-empty mt-4">
            只显示前 {{ visibleWords.length }} 个（共 {{ filteredWords.length }} 个匹配）；用搜索或类目收窄能更快找到。
            <NButton size="small" class="ml-2" data-role="content-filter-show-more" @click="showMoreWords">
              再显示 {{ Math.min(hiddenWordCount, RENDER_WINDOW) }} 个
            </NButton>
          </div>
        </NCard>
      </NTabPane>

      <NTabPane name="whitelist" :tab="`白名单（${whitelistTotal}）`">
        <NCard class="admin-card mb-4" size="small" title="短语白名单">
          <NAlert type="warning" :bordered="false" class="mb-4">
            <strong>只放过这一种说法，词本身照旧拦。</strong>
            被短语<strong>完整包住</strong>的那次命中不替换，但仍会记一条命中（用于统计误伤）：
            加「安全套接层」，正文里的安全套接层不打星，而真写「安全套」仍然打星；
            加「fork 炸弹」，「制作炸弹」照旧打星。
          </NAlert>
          <NAlert type="default" :bordered="false" class="mb-4">
            两点边界：① 只覆盖<strong>已知的连续写法</strong> —— 出现新变体（如「安全套接字层」）要另加一条；
            ② 短语越长豁免范围越大，会连带放过其中出现的其它敏感词，所以别拿整句话当短语。
          </NAlert>
          <NFormItem label="短语（回车添加）">
            <NDynamicTags
              v-model:value="config.whitelist_phrases"
              data-role="content-filter-whitelist-phrases" />
          </NFormItem>
          <NButton type="primary" :loading="saving" @click="save">保存短语白名单</NButton>
        </NCard>

        <NCard class="admin-card mb-4" size="small" title="词级白名单">
          <NAlert type="info" :bordered="false" class="mb-4">
            <strong>整个词都不再打星</strong>（命中仍记录）。只给「正常用法远多于违规用法」的词用，
            例如「独立」「证书」「验证码」—— 加进来等于<strong>放弃这个词的拦截力</strong>，
            想保留拦截力请用上面的短语白名单。
          </NAlert>
          <NFormItem label="词条（回车添加）">
            <NDynamicTags v-model:value="config.whitelist" data-role="content-filter-whitelist" />
          </NFormItem>
          <NButton type="primary" :loading="saving" @click="save">保存白名单</NButton>
        </NCard>
      </NTabPane>

      <NTabPane name="retention" tab="命中记录保留">
        <NCard class="admin-card mb-4" size="small" title="命中记录保留期">
          <NAlert type="info" :bordered="false" class="mb-4">
            关键词风险记录（命中词 + 命中位置前后 20 字上下文）保留多久由这里决定，
            默认 90 天。清理任务每天读一次现值，改完保存即生效，不需要重启。
            记录是<strong>物理删除</strong>：过期之后连上下文片段都不在库里了。
          </NAlert>
          <NFormItem label="保留天数（1–3650）">
            <NInputNumber
              v-model:value="config.retain_days"
              :min="1"
              :max="3650"
              class="admin-field--number"
              data-role="content-filter-retain-days" />
          </NFormItem>
          <NButton type="primary" :loading="saving" @click="save">保存保留期</NButton>
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








