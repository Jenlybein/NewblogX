<script setup lang="ts">
/**
 * 文章发布（`/content/publish`）：只改 site 段 `article` 块的**发布与展示**字段。
 *
 * 2026-11-27 拆页（ADR-0029 第三版）：这一页只剩"谁能发、发出来对谁可见"三个开关；
 * 内容审核 / 投稿配额 / 发布质量闸门搬到了「站点配置 → 文章审核设置」
 * （`/site/article-review`）。两页共写同一个 `article` 块，保存走**字段级补丁**，
 * 兄弟页的字段原样保留。
 *
 * 页面在更早一轮从 `/site/publish` 搬到这里，旧地址留了跳转页。
 */
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInputNumber,
  NSwitch,
  useMessage,
} from "naive-ui";

import {
  getSiteRuntimeConfig,
  updateSiteRuntimeConfig,
} from "~/services/admin";
import type { SiteRuntimeConfig } from "~/types/api";
import { applySiteBlockFieldPatch } from "~/utils/siteSectionSave";

const message = useMessage();
const loading = ref(false);
const saving = ref(false);
const config = ref({} as SiteRuntimeConfig);
const loaded = ref(false);

/** 本页拥有的 `article` 块内字段（保存时只覆盖这些）。 */
const OWNED_ARTICLE_FIELDS = [
  "allow_non_admin_publish",
  "allow_non_admin_draft",
  "show_non_admin_articles",
  "max_tags_per_article",
] as const;

/** 标签上限的兜底值与后端 site_default_settings.yaml 的默认值一致。 */
const DEFAULT_MAX_TAGS = 5;

/** 老库合并默认值之前这一段可能缺键，表单不能因为 undefined 整页炸掉。 */
const maxTags = ref(DEFAULT_MAX_TAGS);

async function load() {
  loading.value = true;
  try {
    config.value = await getSiteRuntimeConfig();
    // 上限是后加的字段：老库合并默认值之前可能是 undefined。
    maxTags.value = config.value.article?.max_tags_per_article ?? DEFAULT_MAX_TAGS;
    loaded.value = true;
  } catch {
    message.error("文章发布配置加载失败");
  } finally {
    loading.value = false;
  }
}

async function save() {
  saving.value = true;
  try {
    if (!config.value.article) {
      message.error("配置未加载完成，请刷新页面后重试");
      return;
    }
    // 读-改-写：取最新整段 → 只覆盖本页字段 → 写回。
    // 本地 ref 在提交前写回配置：读出来的兜底值不能只活在前端。
    config.value.article.max_tags_per_article = maxTags.value;
    const latest = await getSiteRuntimeConfig();
    const merged = applySiteBlockFieldPatch(
      latest,
      config.value,
      "article",
      OWNED_ARTICLE_FIELDS,
    );
    await updateSiteRuntimeConfig(merged);
    message.success("文章发布配置已保存");
  } catch (error) {
    message.error(
      error instanceof Error ? `保存失败：${error.message}` : "保存失败",
    );
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <div class="admin-settings-panel">
    <NAlert type="info" :bordered="false" class="mb-4">
      这一页管「谁能发布、发出来对谁可见」。审核规则（免审核、投稿配额、质量闸门）
      在「站点配置 → 文章审核设置」，两页互不覆盖。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NCard class="admin-card mb-4" title="发布与展示" size="small">
        <NFormItem label="允许非管理员发布文章">
          <NSwitch v-model:value="config.article.allow_non_admin_publish" />
        </NFormItem>
        <NFormItem label="允许非管理员创建草稿">
          <NSwitch v-model:value="config.article.allow_non_admin_draft" />
        </NFormItem>
        <NFormItem label="显示非管理员发布文章">
          <NSwitch v-model:value="config.article.show_non_admin_articles" />
        </NFormItem>
      </NCard>

      <!-- 发布约束（2026-11-27）：标签上限从"代码里写死 5"改成站点配置可调。 -->
      <NCard class="admin-card mb-4" title="发布约束" size="small">
        <NAlert type="info" :bordered="false" class="mb-4">
          作者在写稿页选标签时，超过这个数量会被拦住；这与「标签管理」里的词库是两件事
          —— 那里管"有哪些标签"，这里管"一篇能用几个"。
        </NAlert>
        <NFormItem label="一篇文章最多可选标签数">
          <NInputNumber v-model:value="maxTags" class="admin-field--number" :min="1" :max="20" />
        </NFormItem>
      </NCard>

      <!-- 发布约束（2026-11-27）：标签上限从"代码里写死 5"改成站点配置可调。 -->
      <NCard class="admin-card mb-4" title="发布约束" size="small">
        <NAlert type="info" :bordered="false" class="mb-4">
          作者在写稿页选标签时，超过这个数量会被拦住；这与「标签管理」里的词库是两件事
          —— 那里管"有哪些标签"，这里管"一篇能用几个"。
        </NAlert>
        <NFormItem label="一篇文章最多可选标签数">
          <NInputNumber v-model:value="maxTags" class="admin-field--number" :min="1" :max="20" />
        </NFormItem>
      </NCard>

      <NButton type="primary" :loading="saving" @click="save"
        >保存文章发布配置</NButton
      >
    </NForm>
    <div v-else class="admin-empty">
      {{ loading ? "配置加载中" : "配置加载失败，刷新页面重试" }}
    </div>
  </div>
</template>
