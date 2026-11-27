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
] as const;

async function load() {
  loading.value = true;
  try {
    config.value = await getSiteRuntimeConfig();
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

      <NButton type="primary" :loading="saving" @click="save"
        >保存文章发布配置</NButton
      >
    </NForm>
    <div v-else class="admin-empty">
      {{ loading ? "配置加载中" : "配置加载失败，刷新页面重试" }}
    </div>
  </div>
</template>
