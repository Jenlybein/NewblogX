import { computed } from "vue";

import { useSiteStore } from "~/stores/site";
import {
  articleWritePolicy,
  editorReadOnly,
  writeEntryVisible,
  writePolicyNotices,
} from "~/utils/writePolicy";

/**
 * 读者站对站点「发布与展示」两个写作开关的消费入口（#64）。
 *
 * 只有一处读站点配置：组件不再各自去翻 `runtimeConfig.article`，
 * 否则"发布关了就隐藏入口"这类判断会散到页头、个人中心、写作页三份。
 *
 * 顺带补一次公开引导取数：创作页（`layout: write`）不经过 default/studio 布局，
 * 不在引导批次里，直接打开编辑器时配置会是空的。store 内部有单飞闸门，
 * 与布局里的调用重复也不会多发请求。
 */
export function useArticleWritePolicy() {
  const siteStore = useSiteStore();
  void siteStore.ensurePublicBootstrap();

  const policy = computed(() => articleWritePolicy(siteStore.runtimeConfig));
  const entryVisible = computed(() => writeEntryVisible(policy.value));
  const readOnly = computed(() => editorReadOnly(policy.value));
  const notices = computed(() => writePolicyNotices(policy.value));

  return { policy, entryVisible, readOnly, notices };
}
