<script setup lang="ts">
/**
 * 关键词命中处理（`/site/article-review/keywords`）：命中之后文章去哪。
 *
 * 2026-11-28 从词表页的「命中词自动处置」卡搬来，成为「文章审核设置」的子页。
 *
 * 为什么单独一页：它写的是 content_filter 段的 `auto_dispose`，而那个段还归词表页
 * 所有（词表 / 白名单 / 保留期）。一页两段 → 一个页面只能碰一段是 ADR-0029 的原则，
 * 于是把"命中之后怎么处置"（发布决策）与"词表长什么样"（内容素材）分开住。
 *
 * 三档语义（串行链路的第①环）：命中且非「放行」时，后面的质量评分与 AI 合规判定
 * **全部短路**，不再花钱 —— 结论已经定了。
 */
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NSelect,
  NSpace,
  useMessage,
} from "naive-ui";

import {
  getContentFilterRuntimeConfig,
  updateArticleReviewConfig,
} from "~/services/admin";
import type { HitDisposition, WatchHitDisposition } from "~/types/api";

const message = useMessage();
const loading = ref(false);
const saving = ref(false);

const blockAction = ref<HitDisposition>("release");
const watchAction = ref<WatchHitDisposition>("release");
const loaded = ref(false);

/** 拦截词三档。每档的落点必须写清，否则"退回草稿"与"禁用"没人分得清。 */
const BLOCK_OPTIONS: Array<{
  label: string;
  value: HitDisposition;
  description: string;
}> = [
  {
    label: "放行（打星后照常发布）",
    value: "release",
    description: "命中词替换成等长 *，文本照常入库，继续走质量与 AI 合规判定。",
  },
  {
    label: "退回草稿",
    value: "draft",
    description: "文章落草稿、作者改完可重投；提交配额不归还；不再评质量、不走 AI。",
  },
  {
    label: "禁用文章（落已拒绝）",
    value: "reject",
    description: "文章落「已拒绝」且不可改；不再评质量、不走 AI；读者看不到。",
  },
];

/** 观察词两档：观察档是误报代价低的词，不设重罚。 */
const WATCH_OPTIONS: Array<{
  label: string;
  value: WatchHitDisposition;
  description: string;
}> = [
  {
    label: "放行（打星后照常发布）",
    value: "release",
    description: "命中词替换成等长 *，文本照常入库并继续后续链路。",
  },
  {
    label: "退回草稿",
    value: "draft",
    description: "文章落草稿，作者改完可重投；不再评质量、不走 AI。",
  },
];

const blockDescription = computed(
  () => BLOCK_OPTIONS.find((o) => o.value === blockAction.value)?.description ?? "",
);
const watchDescription = computed(
  () => WATCH_OPTIONS.find((o) => o.value === watchAction.value)?.description ?? "",
);

async function load() {
  loading.value = true;
  try {
    const config = await getContentFilterRuntimeConfig();
    // 老库的段里可能没有这一段：兜 release（改造前的 mask 就是它）。
    blockAction.value = (config.auto_dispose?.block_action as HitDisposition) ?? "release";
    watchAction.value =
      (config.auto_dispose?.watch_action as WatchHitDisposition) ?? "release";
    loaded.value = true;
  } catch {
    message.error("关键词命中处理加载失败");
  } finally {
    loading.value = false;
  }
}

async function save() {
  saving.value = true;
  try {
    // 只传 auto_dispose：这一页是 content_filter 段的**字段级补丁**，
    // 词表 / 白名单 / 保留期由别的页拥有，一条都不该被这次保存覆盖。
    await updateArticleReviewConfig({
      auto_dispose: {
        block_action: blockAction.value,
        watch_action: watchAction.value,
      },
    });
    message.success("关键词命中处理已保存，下一次提交即生效");
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
      命中之后文章去哪，由这里决定。它是发布链路的<strong>第一环</strong>：
      命中且不是「放行」时，后面的<strong>质量评分</strong>与
      <strong>AI 合规判定</strong>立即短路，不再调用模型、不再花额度。
    </NAlert>

    <NForm v-if="loaded" label-placement="top" :disabled="loading">
      <NCard class="admin-card mb-4" title="拦截词命中" size="small">
        <NFormItem label="拦截词（明确违法 / 高危）命中时">
          <NSelect
            v-model:value="blockAction"
            class="admin-field--select"
            :options="BLOCK_OPTIONS"
            data-role="hit-disposition-block"
          />
        </NFormItem>
        <NAlert type="info" :bordered="false" class="mb-4">{{ blockDescription }}</NAlert>
      </NCard>

      <NCard class="admin-card mb-4" title="观察词命中" size="small">
        <NFormItem label="观察词（推广话术为主）命中时">
          <NSelect
            v-model:value="watchAction"
            class="admin-field--select"
            :options="WATCH_OPTIONS"
            data-role="hit-disposition-watch"
          />
        </NFormItem>
        <NAlert type="info" :bordered="false" class="mb-4">{{ watchDescription }}</NAlert>
        <NAlert type="warning" :bordered="false">
          观察档没有「禁用」：观察词是误报代价低的词（推广话术、口头语），
          拒发它们等于把「加微信」三个字写成违规内容。
        </NAlert>
      </NCard>

      <NCard class="admin-card mb-4" title="评论怎么落地" size="small">
        <NAlert type="info" :bordered="false">
          评论只有<strong>两档语义</strong>：选「放行」就打星照常发布；选「退回草稿」或
          「禁用」都等价于<strong>拒收</strong>（不入库、提示作者修改）。
          评论没有草稿态，也没有申诉通道 —— 把它放进待审队列等于让作者以为发出去了
          却永远看不到。昵称 / 简介 / 私信 / 合集仍然只打星。
        </NAlert>
      </NCard>

      <NSpace>
        <NButton type="primary" :loading="saving" @click="save">
          保存命中处理
        </NButton>
      </NSpace>
    </NForm>
  </div>
</template>
