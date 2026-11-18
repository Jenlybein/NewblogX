import { computed, ref, toValue, watch, type MaybeRefOrGetter } from "vue";

/**
 * 「这张卡到底有没有封面」的唯一判据（票 440）。
 *
 * 口径：**没封面就不画封面框**。缺封面、封面只剩空白、封面加载失败，三种情况都算"没有"，
 * 卡片直接换成无封面的排版（信息流里正文占满整行、网格卡里标题顶到上边），
 * 而不是留一块空白的 16:9 框——那块框读起来像"图挂了"，用户已经明确否掉。
 *
 * 为什么要提到消费方而不是留在 `ArticleCover` 里：拿掉封面框会改变**外层排版**
 * （栅格列数、卡片高度）。只有消费方知道该把正文往哪儿挪，所以"有没有封面"
 * 必须是消费方能读到的响应式状态；图片失败的信号由 `ArticleCover` 的 `@fail` 回抛。
 */
export function useCoverVisibility(src: MaybeRefOrGetter<string | null | undefined>) {
  const normalizedSrc = computed(() => (toValue(src) ?? "").trim());
  const failed = ref(false);

  // 换封面时失败态复位：上一张的失败不该把下一张也判成"没有封面"。
  watch(normalizedSrc, () => {
    failed.value = false;
  });

  const hasCover = computed(() => normalizedSrc.value.length > 0 && !failed.value);

  function markCoverFailed(): void {
    failed.value = true;
  }

  return { normalizedSrc, hasCover, markCoverFailed };
}
