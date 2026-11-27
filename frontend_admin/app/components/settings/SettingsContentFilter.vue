<script setup lang="ts">
/**
 * 关键词过滤（`/content/keywords`）：`content_filter` 段的词表与保留期（ADR-0032）。
 *
 * 2026-11-27 按新一代设计稿整页重排：视觉唯一来源是
 * `design/jellybin-replica/admin-content-filter.html` 与 `app.css` 的 `.cf-*` 段
 * （版式数值原样移植，颜色映射到后台语义 token，见 `assets/css/content-filter.css`）。
 *
 * 三件事必须一起说清楚，否则运营会按旧的两份词表习惯乱填：
 *
 *   1. **两档都替换**：拦截词与观察词命中后都变成等长 `*`，颜色只用于标识轻重；
 *   2. **匹配前会归一化**：全角、零宽字符、插空格与分隔符都拦得住，所以词条本身
 *      按正常写法填即可，不需要把变体逐个加进去；
 *   3. **保存即生效**：后端写完缓存就按新词表重建过滤引擎，不需要重启。
 *
 * 白名单已拆到独立页 `/content/keywords/whitelist`：两页共写同一个 `content_filter`
 * 段，**本页只覆盖 `words` + `retain_days` + `auto_dispose`**，白名单字段保存时从最新整段原样带回
 * （`utils/contentFilterSave.ts`）。任意一页只提交自己那几项都会把另一页清空。
 *
 * 词云：字号 = 近 90 天命中处数、颜色 = 档位。次数来自
 * `GET /content-filter/word-stats`（窗口由后端定，响应回显 days，前端不自算 90）。
 * 统计读不到时不影响本页：词表照常可编辑，字号退化成统一 14px（不伪造差异）。
 */
import { NButton, NInput, NInputNumber, NModal, NSelect, useMessage } from "naive-ui";
import ContentFilterTabs from "~/components/settings/ContentFilterTabs.vue";
import {
  getContentFilterRuntimeConfig,
  getContentFilterWordStats,
  updateContentFilterRuntimeConfig,
} from "~/services/admin";
import { isApiBusinessError } from "~/services/http/errors";
import type {
  ContentFilterRuntimeConfig,
  ContentFilterRuntimeConfigPayload,
  ContentFilterWord,
} from "~/types/api";
import type { ContentDisposeAction } from "~/utils/contentFilterSave";
import {
  maskContentFilterText,
  type ContentFilterPreviewResult,
} from "~/utils/contentFilterPreview";
import {
  CONTENT_FILTER_WORD_FIELDS,
  countWordTableChanges,
  mergeContentFilterPayload,
} from "~/utils/contentFilterSave";

type Tier = ContentFilterWord["tier"];
type Category = ContentFilterWord["category"];
type ActiveTab = "words" | "retention";

const route = useRoute();
const message = useMessage();
const loading = ref(false);
const saving = ref(false);
const loaded = ref(false);
// 初值只是占位：表单在 loaded 之前不渲染。
const config = ref({} as ContentFilterRuntimeConfig);

// 保留期在词表页是本地 tab；白名单页跳回本页时用 `?tab=retention` 打开它。
const activeTab = ref<ActiveTab>(route.query.tab === "retention" ? "retention" : "words");
const keyword = ref("");
const categoryFilter = ref("all");
const tierFilter = ref("all");
const hitCounts = ref<Record<string, number>>({});
// 窗口天数由后端响应回显：页面上要显示"最近 N 天"，N 不该在这里写死。
const hitWindowDays = ref(90);

// 过滤示例：纯前端演示，输入只在本页内存里，不发请求。
const sampleText = ref("有需要的私我，代开发票，加微信，量大从优。");

/** 保存前的快照：脏标记按"相对上一次落库的状态"数，而不是"有没有点过按钮"。 */
const baselineWords = ref<ContentFilterWord[]>([]);
const baselineRetainDays = ref(90);

/** 命中词的两个处置动作：打星（现状）/ 拒发（opt-in，只作用于文章面）。 */
const DISPOSE_OPTIONS: { label: string; value: ContentDisposeAction }[] = [
  { label: "只打星（照常发布）", value: "mask" },
  { label: "拒发（不保存，提示作者修改）", value: "reject" },
];

/** 段里缺 auto_dispose 时（老库）的默认值：两档都打星，也就是改造前的行为。 */
const DEFAULT_AUTO_DISPOSE: ContentFilterRuntimeConfigPayload["auto_dispose"] = {
  block_action: "mask",
  watch_action: "mask",
};

/** 自动处置的基线快照：脏标记也要算它，否则改完处置不会提示"未保存"。 */
const baselineAutoDispose = ref({ ...DEFAULT_AUTO_DISPOSE });

const TIER_OPTIONS: { label: string; value: Tier }[] = [
  { label: "拦截词（明确违法/高危）", value: "block" },
  { label: "观察词（推广话术为主）", value: "watch" },
];
const CATEGORY_OPTIONS: { label: string; value: Category }[] = [
  { label: "毒品", value: "drugs" },
  { label: "赌博", value: "gambling" },
  { label: "诈骗", value: "fraud" },
  { label: "色情", value: "porn" },
  { label: "假证发票", value: "fake_docs" },
  { label: "涉政", value: "politics" },
  { label: "暴恐", value: "violence" },
];
const TIER_LABEL: Record<string, string> = { block: "拦截词", watch: "观察词" };
const CATEGORY_LABEL: Record<string, string> = Object.fromEntries(
  CATEGORY_OPTIONS.map((option) => [option.value, option.label]),
);

const words = computed<ContentFilterWord[]>(() => config.value.words ?? []);
const whitelist = computed<string[]>(() => config.value.whitelist ?? []);
const whitelistPhrases = computed<string[]>(
  () => config.value.whitelist_phrases ?? [],
);
// 计数用后端算好的口径（去重后才对得上 `-filter-coverage` 打印的数）；
// 前端自己把两个数组相加会得到"页面 50、体检 49"。
const whitelistTotal = computed(() => config.value.whitelist_stats?.total ?? 0);

/**
 * 必需非空的类目（与后端 contentfilter.RequiredCategories 同一口径）。
 *
 * 清单由**后端下发**（content_filter 段响应里的 required_categories /
 * missing_required_categories），前端不再自己抄一份类目列表：抄一份就等于同一件事
 * 有了两个真相，改一处忘另一处时，红条会在一台机器上安静、在另一台上尖叫。
 */
const missingRequiredCategories = computed<string[]>(
  () => config.value.missing_required_categories ?? [],
);

function categoryLabel(category: string): string {
  return CATEGORY_LABEL[category] ?? category;
}

/** 窗口内命中处数（来自 word-stats；没有命中记录时为 0）。 */
function hitCountOf(word: string): number {
  return hitCounts.value[word] ?? 0;
}

const maxHit = computed(() =>
  words.value.reduce((max, entry) => Math.max(max, hitCountOf(entry.word)), 0),
);

/**
 * 字号 = 命中次数。次数全为 0 时统一 14px（不伪造差异）；
 * 有数据后按最热词线性映射到 14–40px，保证最热词不会被压成一行小字。
 */
function fontSizeOf(word: string): string {
  const count = hitCountOf(word);
  if (maxHit.value <= 0 || count <= 0) return "14px";
  return `${14 + Math.round((26 * count) / maxHit.value)}px`;
}

const filteredWords = computed(() => {
  const needle = keyword.value.trim().toLowerCase();
  return words.value.filter((entry) => {
    if (tierFilter.value !== "all" && entry.tier !== tierFilter.value) return false;
    if (
      categoryFilter.value !== "all" &&
      entry.category !== categoryFilter.value
    )
      return false;
    if (!needle) return true;
    return entry.word.toLowerCase().includes(needle);
  });
});

/**
 * 词云只渲染前 `renderLimit` 个。
 *
 * 词表已经有 6 万条：一次性把 6 万个 <button> 塞进 DOM，浏览器会卡死好几秒，
 * 而运营真正要看的是"搜索 + 按类目/档位收窄后的那几十个"。所以默认只渲染一窗，
 * 底部给出"还有 N 个"并允许逐窗展开；收窄条件变化时窗口回到第一页。
 */
const RENDER_WINDOW = 600;
const renderLimit = ref(RENDER_WINDOW);
const visibleWords = computed(() =>
  filteredWords.value.slice(0, renderLimit.value),
);
const hiddenWordCount = computed(() =>
  Math.max(0, filteredWords.value.length - visibleWords.value.length),
);

function showMoreWords() {
  renderLimit.value += RENDER_WINDOW;
}

// 收窄条件变了就把窗口收回第一页，否则"搜到 3 个但窗口还停在 6000"会让人以为没搜到。
watch([keyword, categoryFilter, tierFilter], () => {
  renderLimit.value = RENDER_WINDOW;
});

const blockCount = computed(
  () => words.value.filter((entry) => entry.tier === "block").length,
);
const watchCount = computed(
  () => words.value.filter((entry) => entry.tier === "watch").length,
);

const categoryCounts = computed<Record<string, number>>(() => {
  const counts: Record<string, number> = {};
  for (const entry of words.value) {
    counts[entry.category] = (counts[entry.category] ?? 0) + 1;
  }
  return counts;
});

function categoryCount(category: string): number {
  return categoryCounts.value[category] ?? 0;
}

const maxCategoryCount = computed(() =>
  CATEGORY_OPTIONS.reduce(
    (max, option) => Math.max(max, categoryCount(option.value)),
    0,
  ),
);

/**
 * 比例条宽度：按最大类目线性映射到 100%，零值留 3% 的"缺口条"（设计稿口径）。
 * 零值不画 0 宽 —— 空白条读起来像"渲染失败"，而 3% + 红点数才读得出"这一类是空的"。
 */
function categoryWidth(category: string): string {
  const count = categoryCount(category);
  if (maxCategoryCount.value <= 0 || count <= 0) return "3%";
  return `${Math.max(3, Math.round((100 * count) / maxCategoryCount.value))}%`;
}

function percentOf(count: number, total: number): string {
  if (total <= 0 || count <= 0) return "0%";
  return `${Math.round((100 * count) / total)}%`;
}

/** 过滤示例：本地按当前词表 + 两份白名单打星，口径见 utils/contentFilterPreview.ts。 */
const samplePreview = computed<ContentFilterPreviewResult>(() =>
  maskContentFilterText(
    sampleText.value,
    words.value,
    whitelist.value,
    whitelistPhrases.value,
  ),
);

/**
 * 自动处置是否被改过。
 *
 * 只比两个动作字段：后端未来可能给这段加别的字段，那些字段的改动不该让本页的
 * 脏标记亮起来（本页只拥有这两个动作）。
 */
const disposeChanged = computed(() => {
  const current = config.value.auto_dispose ?? DEFAULT_AUTO_DISPOSE;
  return (
    current.block_action !== baselineAutoDispose.value.block_action ||
    current.watch_action !== baselineAutoDispose.value.watch_action
  );
});

/** 未保存改动数：词表逐条比较 + 保留期 + 自动处置。 */
const dirtyCount = computed(() => {
  const retainDays = config.value.retain_days ?? 90;
  return (
    countWordTableChanges(baselineWords.value, words.value) +
    (retainDays !== baselineRetainDays.value ? 1 : 0) +
    (disposeChanged.value ? 1 : 0)
  );
});

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
    // 老库的段里可能没有这一段：兜默认值，表单才不会因为 undefined 整页炸掉。
    auto_dispose: current.auto_dispose ?? { ...DEFAULT_AUTO_DISPOSE },
  };
  baselineWords.value = words.value.map((entry) => ({ ...entry }));
  baselineRetainDays.value = config.value.retain_days;
  baselineAutoDispose.value = { ...config.value.auto_dispose };
}

async function load() {
  loading.value = true;
  try {
    applyConfig(await getContentFilterRuntimeConfig());
    loaded.value = true;
  } catch {
    message.error("内容过滤配置加载失败");
  } finally {
    loading.value = false;
  }
  // 词云统计单独拉：它失败不该让整页打不开（词表编辑才是本页的主功能）。
  await loadWordStats();
}

/** 拉一次词云统计；失败只静默降级，不清空既有值。 */
async function loadWordStats() {
  try {
    const stats = await getContentFilterWordStats();
    hitWindowDays.value = stats.days || 90;
    hitCounts.value = Object.fromEntries(
      stats.items.map((item) => [item.word, item.count]),
    );
  } catch {
    // 统计是辅助展示：静默退化成"全部 0 次"，不打断词表编辑。
  }
}

function selectTab(tab: ActiveTab) {
  activeTab.value = tab;
}

/**
 * 保存本页拥有的字段（词表 + 保留期）。
 *
 * 关键在"取最新整段再只覆盖本页字段"：白名单已拆到独立页，两页共写
 * `content_filter` 段，后端对 PUT 是整段覆盖（缺字段 = 零值），直接把
 * 本页的旧快照写回去会把兄弟页刚存的白名单清空。
 */
async function save() {
  saving.value = true;
  try {
    const latest = await getContentFilterRuntimeConfig();
    const payload = mergeContentFilterPayload(
      latest,
      {
        words: words.value,
        retain_days: config.value.retain_days,
        auto_dispose: config.value.auto_dispose ?? DEFAULT_AUTO_DISPOSE,
      },
      CONTENT_FILTER_WORD_FIELDS,
    );
    await updateContentFilterRuntimeConfig(payload);
    // 保存后重新拉一次：缺口清单是后端算的，改完词表要按新词表重新判，
    // 否则刚删空某一类的红条要等下次进页面才出现。
    applyConfig(await getContentFilterRuntimeConfig());
    message.success(
      activeTab.value === "retention"
        ? "保留期已保存；清理任务每天读一次现值，立即生效"
        : "词表已保存；过滤器按新词表重建，立即生效",
    );
  } catch (error) {
    // 必须原样显示后端文案：词表保存失败的原因几乎总是"某个词条不合法"
    // （档位/类目不认识、单字词），只说一句"保存失败"会让运营在 6 万条词表里
    // 反复试同一份数据 —— 2026-11-27 就发生过一次，定位只能靠翻日志。
    message.error(
      isApiBusinessError(error)
        ? error.message
        : "保存失败：词表未能写入，请刷新页面后重试",
    );
  } finally {
    saving.value = false;
  }
}

// ── 点词编辑 ────────────────────────────────────────────────
// 编辑态用 string 承载下拉值：naive-ui 的 v-model 给的是 string，
// 直接绑到契约里窄化成联合类型的字段上会让 vue-tsc 报错。
const editing = ref<ContentFilterWord | null>(null);
// 打开编辑时的原词：编辑框里的文字会被改，定位/删除都要用改之前的那一个。
const editingKey = ref("");
const editTier = ref("watch");
const editCategory = ref("fraud");

function openEditor(entry: ContentFilterWord) {
  editing.value = { ...entry };
  editingKey.value = entry.word;
  editTier.value = entry.tier;
  editCategory.value = entry.category;
}

function closeEditor() {
  editing.value = null;
}

function applyEdit() {
  const entry = editing.value;
  if (!entry) return;
  const word = entry.word.trim();
  if (!word) {
    message.warning("词条不能为空");
    return;
  }
  if (!isTier(editTier.value) || !isCategory(editCategory.value)) {
    message.warning("档位或类目不合规");
    return;
  }
  const duplicated = words.value.some(
    (existing) =>
      existing.word !== editingKey.value &&
      existing.word.toLowerCase() === word.toLowerCase(),
  );
  if (duplicated) {
    message.warning(`词条「${word}」已经在表里了`);
    return;
  }
  const index = words.value.findIndex(
    (existing) => existing.word === editingKey.value,
  );
  if (index < 0) return;
  words.value[index] = {
    word,
    tier: editTier.value,
    category: editCategory.value,
  };
  closeEditor();
}

function removeEditing() {
  const entry = editing.value;
  if (!entry) return;
  config.value.words = words.value.filter(
    (existing) => existing.word !== editingKey.value,
  );
  closeEditor();
}

// ── 批量导入（每行一词）────────────────────────────────────
const importing = ref(false);
const importText = ref("");
const importTier = ref("watch");
const importCategory = ref("fraud");

function openImport() {
  importText.value = "";
  importing.value = true;
}

function applyImport() {
  if (!isTier(importTier.value) || !isCategory(importCategory.value)) {
    message.warning("档位或类目不合规");
    return;
  }
  const existing = new Set(
    words.value.map((entry) => entry.word.toLowerCase()),
  );
  const added: ContentFilterWord[] = [];
  let skipped = 0;
  for (const line of importText.value.split(/\r?\n/u)) {
    const word = line.trim();
    if (!word) continue;
    if (runeLength(word) < 2) {
      skipped += 1;
      continue;
    }
    const key = word.toLowerCase();
    if (existing.has(key)) {
      skipped += 1;
      continue;
    }
    existing.add(key);
    added.push({
      word,
      tier: importTier.value,
      category: importCategory.value,
    });
  }
  config.value.words = [...words.value, ...added];
  importing.value = false;
  message.success(
    `已导入 ${added.length} 条，跳过 ${skipped} 条（重复或单字）`,
  );
}

function isTier(value: string): value is Tier {
  return value === "block" || value === "watch";
}

function isCategory(value: string): value is Category {
  return CATEGORY_OPTIONS.some((option) => option.value === value);
}

/** 位图里一个汉字算 1："词条太短"这条判据按字符数而不是字节数。 */
function runeLength(value: string): number {
  return Array.from(value).length;
}

function onEditorShow(show: boolean) {
  if (!show) closeEditor();
}

function onImportShow(show: boolean) {
  importing.value = show;
}

const categoryFilterOptions = computed(() => [
  { label: "全部类目", value: "all" },
  ...CATEGORY_OPTIONS.map((option) => ({
    label: option.label,
    value: option.value,
  })),
]);

const tierFilterOptions = [
  { label: "全部档位", value: "all" },
  { label: "只看拦截", value: "block" },
  { label: "只看观察", value: "watch" },
];

onMounted(load);
</script>

<template>
  <div class="cf-page">
    <div class="cf-head">
      <div class="cf-head__bar">
        <div>
          <h2 class="cf-head__title">关键词过滤</h2>
          <p class="cf-head__note">
            全站唯一词表：文章、评论、昵称、简介、私信、合集标题与描述都用它。拦截词与观察词命中后
            <strong>都替换成等长的 <code>*</code></strong
            >（颜色只用来分轻重），文本照常发布；命中明细进「审核 → 关键词风险」。
            匹配前会做归一化（全角折半角、拉丁转小写、零宽字符当分隔符、空白与标点压掉），
            <strong>不用为变体逐个加词</strong>。词表不会出现在任何匿名接口里，保存后立即生效。
          </p>
        </div>
        <div class="cf-head__actions">
          <span v-if="dirtyCount > 0" class="cf-dirty" data-role="content-filter-dirty">
            未保存 <data :value="dirtyCount">{{ dirtyCount }}</data>
          </span>
          <NButton
            v-if="activeTab === 'words'"
            size="small"
            data-role="content-filter-import"
            @click="openImport"
          >
            批量导入
          </NButton>
          <NButton
            size="small"
            type="primary"
            :loading="saving"
            data-role="content-filter-save"
            @click="save"
          >
            {{ activeTab === "retention" ? "保存保留期" : "保存词表" }}
          </NButton>
        </div>
      </div>

      <ContentFilterTabs
        :active="activeTab"
        :word-count="words.length"
        :whitelist-count="whitelistTotal"
        @select="selectTab"
      />

      <div v-if="activeTab === 'words'" class="cf-toolbar">
        <div class="cf-filter-row">
          <NInput
            v-model:value="keyword"
            placeholder="搜索词条"
            clearable
            class="cf-search admin-field--url"
          />
          <NSelect
            v-model:value="categoryFilter"
            :options="categoryFilterOptions"
            class="cf-filter-select admin-field--select"
          />
          <NSelect
            v-model:value="tierFilter"
            :options="tierFilterOptions"
            class="cf-filter-select admin-field--select"
          />
        </div>
        <div class="cf-toolbar__actions">
          <span class="cf-count cf-count--block"
            >拦截 <data :value="blockCount">{{ blockCount }}</data></span
          >
          <span class="cf-count cf-count--watch"
            >观察 <data :value="watchCount">{{ watchCount }}</data></span
          >
        </div>
      </div>
    </div>

    <div v-if="!loaded" class="cf-empty">
      {{ loading ? "配置加载中" : "配置加载失败，刷新页面重试" }}
    </div>

    <template v-else>
      <section
        v-if="activeTab === 'words'"
        class="cf-sample"
        aria-label="过滤效果"
      >
        <div class="cf-sample__row">
          <span class="cf-sample__key">试一段</span>
          <NInput
            v-model:value="sampleText"
            placeholder="输入一段文字，看它发布后会变成什么样（只在本页演示，不发请求）"
            class="cf-sample__input admin-field--long"
          />
        </div>
        <div class="cf-sample__row">
          <span class="cf-sample__key">发布后</span>
          <p class="cf-sample__text" data-role="content-filter-sample">
            <template v-if="sampleText.trim().length > 0">
              <template
                v-for="(segment, index) in samplePreview.segments"
                :key="index"
              >
                <mark v-if="segment.masked" class="is-masked">{{ segment.text }}</mark>
                <span v-else>{{ segment.text }}</span>
              </template>
              <span class="cf-note">（命中 {{ samplePreview.hits }} 处，只演示当前词表）</span>
            </template>
            <span v-else class="cf-note">先在上面输入一段文字；演示只读本页当前词表，不发请求。</span>
          </p>
        </div>
        <div class="cf-sample__row">
          <span class="cf-sample__key">匹配规则</span>
          <p class="cf-sample__text">
            <code>1氨丙酮</code> <code>１氨丙酮</code> <code>1 氨 丙酮</code>
            会被归一化成同一个词；反过来，两个字的短词更容易误伤，单字词不入表。
          </p>
        </div>
      </section>

      <!-- 类目缺口：涉政与暴恐按设计只进数据库、不进仓库，仓库与 CI 全绿不代表线上词库完整。 -->
      <div
        v-if="activeTab === 'words' && missingRequiredCategories.length > 0"
        class="cf-strip"
        role="status"
      >
        <span class="cf-strip__item"
          >未覆盖类目 <data :value="missingRequiredCategories.length">{{ missingRequiredCategories.length }}</data></span
        >
        <span
          v-for="category in missingRequiredCategories"
          :key="category"
          class="cf-strip__item"
        >
          {{ categoryLabel(category) }} <data :value="0">0</data>
        </span>
        <a class="cf-strip__link" href="#cf-gap">补齐</a>
      </div>

      <section
        v-if="activeTab === 'words'"
        class="cf-layout"
      >
        <div>
          <div class="cf-legend">
            <span><span class="cf-legend__dot cf-legend__dot--block" />拦截</span>
            <span><span class="cf-legend__dot cf-legend__dot--watch" />观察</span>
            <span>字号 = 最近 {{ hitWindowDays }} 天的命中处数；没有记录的统一小字号</span>
          </div>

          <div
            v-if="visibleWords.length > 0"
            class="cf-cloud"
            data-role="content-filter-cloud"
            :data-tier="tierFilter"
            :data-total="words.length"
            :data-shown="visibleWords.length"
          >
            <button
              v-for="entry in visibleWords"
              :key="entry.word"
              type="button"
              class="cf-word"
              :class="
                entry.tier === 'block' ? 'cf-word--block' : 'cf-word--watch'
              "
              :style="{ fontSize: fontSizeOf(entry.word) }"
              :data-tier="entry.tier"
              :data-total="words.length"
              :data-shown="visibleWords.length"
              :title="`${TIER_LABEL[entry.tier]} · ${CATEGORY_LABEL[entry.category]} · 最近 ${hitWindowDays} 天命中 ${hitCountOf(entry.word)} 处`"
              @click="openEditor(entry)"
            >
              {{ entry.word }}
            </button>
          </div>
          <div v-else class="cf-empty">
            {{
              words.length === 0
                ? "词表还是空的：先点「批量导入」贴入词条"
                : "没有匹配的词条"
            }}
          </div>

          <div class="cf-more">
            <span
              >显示 <data :value="visibleWords.length">{{ visibleWords.length }}</data> /
              <data :value="filteredWords.length">{{ filteredWords.length }}</data></span
            >
            <NButton
              v-if="hiddenWordCount > 0"
              size="small"
              data-role="content-filter-show-more"
              @click="showMoreWords"
            >
              再显示 {{ Math.min(hiddenWordCount, RENDER_WINDOW) }} 个
            </NButton>
            <span v-if="hiddenWordCount > 0"
              >词表 {{ words.length }} 条，一次只渲染一窗；用搜索 / 类目 / 档位收窄最省事。</span
            >
            <span class="cf-note"
              >点任意一个词可以改档位、改类目或删除，改动先落在本页，点「保存词表」才写进后端。</span
            >
          </div>
        </div>

        <aside class="cf-aside">
          <div class="cf-panel">
            <h2 class="cf-panel__title">档位</h2>
            <div class="cf-meter">
              <span>拦截</span>
              <span class="cf-meter__track">
                <span
                  class="cf-meter__fill cf-meter__fill--block"
                  :style="{ width: percentOf(blockCount, words.length) }"
                />
              </span>
              <span class="cf-meter__value">{{ blockCount }}</span>
            </div>
            <div class="cf-meter">
              <span>观察</span>
              <span class="cf-meter__track">
                <span
                  class="cf-meter__fill cf-meter__fill--watch"
                  :style="{ width: percentOf(watchCount, words.length) }"
                />
              </span>
              <span class="cf-meter__value">{{ watchCount }}</span>
            </div>
          </div>

          <div id="cf-gap" class="cf-panel">
            <h2 class="cf-panel__title">类目</h2>
            <div
              v-for="option in CATEGORY_OPTIONS"
              :key="option.value"
              class="cf-meter"
            >
              <span>{{ option.label }}</span>
              <span class="cf-meter__track">
                <span
                  class="cf-meter__fill"
                  :class="{
                    'cf-meter__fill--empty': categoryCount(option.value) === 0,
                  }"
                  :style="{ width: categoryWidth(option.value) }"
                />
              </span>
              <span
                class="cf-meter__value"
                :class="{ 'cf-meter__value--zero': categoryCount(option.value) === 0 }"
                >{{ categoryCount(option.value) }}</span
              >
            </div>
            <p v-if="missingRequiredCategories.length > 0" class="cf-note mt-3">
              <strong>{{ missingRequiredCategories.map(categoryLabel).join("、") }}</strong>
              这几类按设计只进数据库、不进仓库，必须由部署侧生成后导入：在有 node 的机器上跑
              <code>scripts/content-filter/build-wordlist.mjs</code> 产出词表，再到部署机用
              <code>server -filter-import</code> 导入并重启服务。导入前这些类目处于完全不过滤状态。
            </p>
          </div>
        </aside>
      </section>

      <section
        v-if="activeTab === 'retention'"
        class="cf-panel max-w-[560px]"
      >
        <h2 class="cf-panel__title">命中记录保留期</h2>
        <p class="cf-note">
          关键词风险记录（命中词 + 命中位置前后 20 字上下文）保留多久由这里决定，默认 90 天。
          清理任务每天读一次现值，改完保存即生效，不需要重启。记录是<strong>物理删除</strong>：
          过期之后连上下文片段都不在库里了。
        </p>
        <div class="cf-filter-row mt-3">
          <span class="cf-sample__key">保留天数（1–3650）</span>
          <NInputNumber
            v-model:value="config.retain_days"
            :min="1"
            :max="3650"
            class="admin-field--number"
            data-role="content-filter-retain-days"
          />
        </div>

        <!-- 命中词自动处置（2026-11-27）：满星替换是今天的唯一处置，拒发是 opt-in。
             两个动作都是"命中之后怎么办"，与保留期同属"设置"这一类，所以放在同一面板。 -->
        <h2 class="cf-panel__title mt-6">命中词自动处置</h2>
        <p class="cf-note">
          命中之后的默认处置是把命中词替换成等长的 <code>*</code>（本文照常发布，命中明细进
          「审核 → 关键词风险」）。把<strong>拦截档</strong>改成「拒发」后，文章标题 / 摘要 / 正文
          一命中就直接拒绝保存（作者看到的是"内容包含不允许发布的词"，不会拿到命中词本身，
          命中记录照旧进风险页）。观察档目前只有「打星」。这套处置<strong>只作用于文章</strong>，
          评论 / 昵称 / 私信 / 合集仍是打星。
        </p>
        <div class="cf-filter-row mt-3">
          <span class="cf-sample__key">拦截档命中</span>
          <NSelect
            v-model:value="config.auto_dispose.block_action"
            :options="DISPOSE_OPTIONS"
            class="admin-field--select"
            data-role="content-filter-dispose-block"
          />
        </div>
        <div class="cf-filter-row mt-3">
          <span class="cf-sample__key">观察档命中</span>
          <NSelect
            v-model:value="config.auto_dispose.watch_action"
            :options="DISPOSE_OPTIONS"
            class="admin-field--select"
            data-role="content-filter-dispose-watch"
          />
        </div>
      </section>
    </template>

    <NModal
      :show="editing !== null"
      preset="card"
      style="width: 420px"
      title="编辑词条"
      @update:show="onEditorShow"
    >
      <template v-if="editing">
        <div class="cf-field">
          <span class="cf-sample__key">词条</span>
          <NInput v-model:value="editing.word" class="admin-field--key" />
        </div>
        <div class="cf-field">
          <span class="cf-sample__key">档位</span>
          <NSelect
            v-model:value="editTier"
            :options="TIER_OPTIONS"
            class="admin-field--select"
          />
        </div>
        <div class="cf-field">
          <span class="cf-sample__key">类目</span>
          <NSelect
            v-model:value="editCategory"
            :options="CATEGORY_OPTIONS"
            class="admin-field--select"
          />
        </div>
      </template>
      <div class="cf-field cf-field--actions">
        <NButton type="error" quaternary @click="removeEditing">删除</NButton>
        <div class="cf-filter-row">
          <NButton @click="closeEditor">取消</NButton>
          <NButton type="primary" @click="applyEdit">确定</NButton>
        </div>
      </div>
    </NModal>

    <NModal
      :show="importing"
      preset="card"
      style="width: 560px"
      title="批量导入词条"
      @update:show="onImportShow"
    >
      <p class="cf-note">
        一行一个词，空行忽略；单字词与重复词会被跳过。导入只加到本页的待保存列表，
        点「保存词表」才会写进后端。
      </p>
      <div class="cf-field mt-3">
        <span class="cf-sample__key">导入后的默认档位</span>
        <NSelect
          v-model:value="importTier"
          :options="TIER_OPTIONS"
          class="admin-field--select"
        />
      </div>
      <div class="cf-field">
        <span class="cf-sample__key">导入后的默认类目</span>
        <NSelect
          v-model:value="importCategory"
          :options="CATEGORY_OPTIONS"
          class="admin-field--select"
        />
      </div>
      <div class="cf-field">
        <span class="cf-sample__key">词条（每行一词）</span>
        <NInput
          v-model:value="importText"
          type="textarea"
          :autosize="{ minRows: 8, maxRows: 16 }"
          class="admin-field--long"
          placeholder="代开发票&#10;办假证&#10;加微信"
        />
      </div>
      <div class="cf-field cf-field--actions">
        <span />
        <div class="cf-filter-row">
          <NButton @click="importing = false">取消</NButton>
          <NButton type="primary" @click="applyImport">导入</NButton>
        </div>
      </div>
    </NModal>
  </div>
</template>

