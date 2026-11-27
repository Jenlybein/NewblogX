<script setup lang="ts">
/**
 * 关键词白名单（`/content/keywords/whitelist`）：`content_filter` 段的两份白名单（ADR-0032）。
 *
 * 2026-11-27 从词表页拆出来：白名单是"例外清单"，和词表是两种心智 —— 词表回答
 * "拦什么"，白名单回答"放过哪次命中"。挤在同一个 tab 里时，运营改词表顺手就把
 * 白名单清空（旧页只有一个保存按钮，语义是整段覆盖）。
 *
 * 视觉唯一来源：`design/jellybin-replica/admin-content-filter-whitelist.html` 与
 * `app.css` 的 `.cf-*` 段；两列「输入 → 发布后」对照由本页按当前词表 + 白名单
 * **本地**算出来（纯函数见 `utils/contentFilterPreview.ts`），不发请求。
 *
 * 保存语义：两页共写 `content_filter` 段，后端对 PUT 是整段覆盖，
 * **本页只覆盖 `whitelist` + `whitelist_phrases`**，词表与保留期从最新整段原样带回
 * （`utils/contentFilterSave.ts`）。反过来，词表页保存时也不会动这两份白名单。
 *
 * 两种粒度必须讲清差别：
 *   - **短语白名单**：只放进被短语完整包住的那次命中，词本身照旧拦；
 *   - **词级白名单**：整个词都不再打星（命中仍记录），等于放弃这个词的拦截力。
 */
import { NButton, NInput, useMessage } from "naive-ui";
import ContentFilterTabs from "~/components/settings/ContentFilterTabs.vue";
import {
  getContentFilterRuntimeConfig,
  updateContentFilterRuntimeConfig,
} from "~/services/admin";
import { isApiBusinessError } from "~/services/http/errors";
import type { ContentFilterRuntimeConfig } from "~/types/api";
import { maskContentFilterText } from "~/utils/contentFilterPreview";
import {
  CONTENT_FILTER_WHITELIST_FIELDS,
  countTagChanges,
  mergeContentFilterPayload,
} from "~/utils/contentFilterSave";

const message = useMessage();
const loading = ref(false);
const saving = ref(false);
const loaded = ref(false);
// 初值只是占位：表单在 loaded 之前不渲染。
const config = ref({} as ContentFilterRuntimeConfig);

// 两份白名单各自绑定成本地数组：表单一改就能立刻算脏标记与下方对照表。
const whitelist = ref<string[]>([]);
const whitelistPhrases = ref<string[]>([]);

/** 保存前的快照：脏标记按"相对上一次落库的状态"数。 */
const baselineWhitelist = ref<string[]>([]);
const baselinePhrases = ref<string[]>([]);

const phraseDraft = ref("");
const wordDraft = ref("");

const words = computed(() => config.value.words ?? []);
const wordCount = computed(() => words.value.length);
// 计数用后端算好的口径（去重后才对得上 `-filter-coverage` 打印的数）。
const whitelistTotal = computed(() => config.value.whitelist_stats?.total ?? 0);

/** 未保存改动数：两份白名单的新增 + 删除条数。 */
const dirtyCount = computed(
  () =>
    countTagChanges(baselineWhitelist.value, whitelist.value) +
    countTagChanges(baselinePhrases.value, whitelistPhrases.value),
);

function applyConfig(current: ContentFilterRuntimeConfig) {
  config.value = {
    ...current,
    words: current.words ?? [],
    whitelist: current.whitelist ?? [],
    whitelist_phrases: current.whitelist_phrases ?? [],
    retain_days: current.retain_days ?? 90,
  };
  whitelist.value = [...config.value.whitelist];
  whitelistPhrases.value = [...config.value.whitelist_phrases];
  baselineWhitelist.value = [...whitelist.value];
  baselinePhrases.value = [...whitelistPhrases.value];
}

async function load() {
  loading.value = true;
  try {
    applyConfig(await getContentFilterRuntimeConfig());
    loaded.value = true;
  } catch {
    message.error("关键词白名单加载失败");
  } finally {
    loading.value = false;
  }
}

/**
 * 保存本页拥有的两份白名单。
 *
 * 读-改-写：取最新整段 → 只覆盖白名单两个字段 → 写回。词表与保留期是
 * 词表页拥有的字段，本页原样带回，绝不能顺手一起提交。
 */
async function save() {
  saving.value = true;
  try {
    const latest = await getContentFilterRuntimeConfig();
    const payload = mergeContentFilterPayload(
      latest,
      {
        whitelist: whitelist.value,
        whitelist_phrases: whitelistPhrases.value,
      },
      CONTENT_FILTER_WHITELIST_FIELDS,
    );
    await updateContentFilterRuntimeConfig(payload);
    // 保存后重拉一次，让页头计数（whitelist_stats）按后端口径刷新。
    applyConfig(await getContentFilterRuntimeConfig());
    message.success("白名单已保存；过滤器按新白名单重建，立即生效");
  } catch (error) {
    // 原样显示后端文案：白名单保存失败的原因（短语为空、重复项等）只有后端知道。
    message.error(
      isApiBusinessError(error)
        ? error.message
        : "保存失败：白名单未能写入，请刷新页面后重试",
    );
  } finally {
    saving.value = false;
  }
}

/** 同一份清单里做大小写不敏感去重（引擎匹配前会折小写）。 */
function isDuplicate(list: readonly string[], value: string): boolean {
  const key = value.toLowerCase();
  return list.some((item) => item.toLowerCase() === key);
}

function addPhrase() {
  const value = phraseDraft.value.trim();
  if (!value) return;
  if (isDuplicate(whitelistPhrases.value, value)) {
    message.warning(`短语「${value}」已经在表里了`);
    return;
  }
  whitelistPhrases.value = [...whitelistPhrases.value, value];
  phraseDraft.value = "";
}

function removePhrase(value: string) {
  whitelistPhrases.value = whitelistPhrases.value.filter((item) => item !== value);
}

function addWord() {
  const value = wordDraft.value.trim();
  if (!value) return;
  if (isDuplicate(whitelist.value, value)) {
    message.warning(`词条「${value}」已经在表里了`);
    return;
  }
  whitelist.value = [...whitelist.value, value];
  wordDraft.value = "";
}

function removeWord(value: string) {
  whitelist.value = whitelist.value.filter((item) => item !== value);
}

interface DiffRow {
  input: string
  output: string
}

/**
 * 本地算一次「输入 → 发布后」：只看这一条在当前两份白名单下的效果。
 * 命中白名单的条目原样返回；不在词表里的条目也原样返回（不伪造打星）。
 */
function previewOf(text: string): string {
  return maskContentFilterText(
    text,
    words.value,
    whitelist.value,
    whitelistPhrases.value,
  ).masked;
}

/**
 * 短语白名单的对照：短语本身 → 原样放行；短语里包着的那个敏感词单独出现
 * → 照旧打星。两行放一起才读得出"只放过这一种说法"。
 */
const phraseRows = computed<DiffRow[]>(() => {
  const rows: DiffRow[] = [];
  for (const phrase of whitelistPhrases.value.slice(0, 2)) {
    rows.push({ input: phrase, output: previewOf(phrase) });
    const inner = words.value.find(
      (entry) =>
        entry.word !== phrase &&
        entry.word.length >= 2 &&
        phrase.toLowerCase().includes(entry.word.toLowerCase()),
    );
    if (inner) rows.push({ input: inner.word, output: previewOf(inner.word) });
  }
  return rows;
});

/**
 * 词级白名单的对照：这个词 → 原样放行；包含它的更长词条 → 照旧打星
 * （说明豁免的是"这个整词"，不是"含有它的所有文本"）。
 */
const wordRows = computed<DiffRow[]>(() => {
  const rows: DiffRow[] = [];
  for (const word of whitelist.value.slice(0, 2)) {
    rows.push({ input: word, output: previewOf(word) });
    const longer = words.value.find(
      (entry) =>
        entry.word.toLowerCase() !== word.toLowerCase() &&
        entry.word.toLowerCase().includes(word.toLowerCase()),
    );
    if (longer) rows.push({ input: longer.word, output: previewOf(longer.word) });
  }
  return rows;
});

onMounted(load);
</script>

<template>
  <div class="cf-page">
    <div class="cf-head">
      <div class="cf-head__bar">
        <div>
          <h2 class="cf-head__title">关键词白名单</h2>
          <p class="cf-head__note">
            命中先过白名单：<strong>短语级</strong>只放过被短语完整包住的那次命中，词本身照旧拦；
            <strong>词级</strong>整个词都不再打星（命中仍记一条风险，用于统计误伤）。
            两页共写同一段配置 —— 保存只写白名单两项，词表与保留期不会被本页改动。
          </p>
        </div>
        <div class="cf-head__actions">
          <span v-if="dirtyCount > 0" class="cf-dirty" data-role="content-filter-whitelist-dirty">
            未保存 <data :value="dirtyCount">{{ dirtyCount }}</data>
          </span>
          <NButton
            size="small"
            type="primary"
            :loading="saving"
            data-role="content-filter-whitelist-save"
            @click="save"
          >
            保存白名单
          </NButton>
        </div>
      </div>

      <ContentFilterTabs
        active="whitelist"
        :word-count="wordCount"
        :whitelist-count="whitelistTotal"
        cross-page
      />
    </div>

    <div v-if="!loaded" class="cf-empty">
      {{ loading ? "配置加载中" : "配置加载失败，刷新页面重试" }}
    </div>

    <section v-else class="cf-pair" aria-label="白名单">
      <div class="cf-panel">
        <h2 class="cf-panel__title">
          <span>短语白名单</span>
          <span class="cf-scope">短语内豁免</span>
        </h2>
        <p class="cf-note">
          <strong>只放过这一种说法，词本身照旧拦。</strong>
          被短语<strong>完整包住</strong>的那次命中不替换，但仍会记一条命中（用于统计误伤）：
          加「安全套接层」，正文里的安全套接层不打星，而真写「安全套」仍然打星；
          加「fork 炸弹」，「制作炸弹」照旧打星。
        </p>
        <p class="cf-note mt-2">
          两点边界：① 只覆盖<strong>已知的连续写法</strong> —— 出现新变体（如「安全套接字层」）
          要另加一条；② 短语越长豁免范围越大，会连带放过其中出现的其它敏感词，所以别拿整句话当短语。
        </p>

        <div class="cf-tagbox mt-3">
          <span
            v-for="phrase in whitelistPhrases"
            :key="phrase"
            class="cf-tag"
          >
            {{ phrase }}
            <button
              type="button"
              class="cf-tag__x"
              :aria-label="`移除短语 ${phrase}`"
              @click="removePhrase(phrase)"
            >
              ×
            </button>
          </span>
          <span v-if="whitelistPhrases.length === 0" class="cf-note">
            还没有短语白名单：在下面加一条，右侧对照会立刻显示「输入 → 发布后」。
          </span>
        </div>

        <table v-if="phraseRows.length > 0" class="cf-diff">
          <thead>
            <tr>
              <th scope="col">文本</th>
              <th scope="col">发布后</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="(row, index) in phraseRows"
              :key="index"
              :class="{ 'is-hit': row.output !== row.input }"
            >
              <td>{{ row.input }}</td>
              <td>
                <mark v-if="row.output !== row.input" class="is-masked">{{ row.output }}</mark>
                <span v-else>{{ row.output }}</span>
              </td>
            </tr>
          </tbody>
        </table>

        <div class="cf-filter-row mt-3">
          <NInput
            v-model:value="phraseDraft"
            placeholder="加短语"
            class="cf-input admin-field--url"
            @keyup.enter="addPhrase"
          />
          <NButton size="small" type="primary" @click="addPhrase">添加短语</NButton>
        </div>
      </div>

      <div class="cf-panel">
        <h2 class="cf-panel__title">
          <span>词级白名单</span>
          <span class="cf-scope">整词豁免</span>
        </h2>
        <p class="cf-note">
          <strong>整个词都不再打星</strong>（命中仍记录）。只给「正常用法远多于违规用法」的词用，
          例如「独立」「证书」「验证码」—— 加进来等于<strong>放弃这个词的拦截力</strong>，
          想保留拦截力请用左边的短语白名单。
        </p>

        <div class="cf-tagbox mt-3">
          <span v-for="word in whitelist" :key="word" class="cf-tag">
            {{ word }}
            <button
              type="button"
              class="cf-tag__x"
              :aria-label="`移除词条 ${word}`"
              @click="removeWord(word)"
            >
              ×
            </button>
          </span>
          <span v-if="whitelist.length === 0" class="cf-note">
            还没有词级白名单：在下面加一条，右侧对照会立刻显示「输入 → 发布后」。
          </span>
        </div>

        <table v-if="wordRows.length > 0" class="cf-diff">
          <thead>
            <tr>
              <th scope="col">文本</th>
              <th scope="col">发布后</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="(row, index) in wordRows"
              :key="index"
              :class="{ 'is-hit': row.output !== row.input }"
            >
              <td>{{ row.input }}</td>
              <td>
                <mark v-if="row.output !== row.input" class="is-masked">{{ row.output }}</mark>
                <span v-else>{{ row.output }}</span>
              </td>
            </tr>
          </tbody>
        </table>

        <div class="cf-filter-row mt-3">
          <NInput
            v-model:value="wordDraft"
            placeholder="加词"
            class="cf-input admin-field--url"
            @keyup.enter="addWord"
          />
          <NButton size="small" type="primary" @click="addWord">添加词条</NButton>
        </div>
      </div>
    </section>
  </div>
</template>
