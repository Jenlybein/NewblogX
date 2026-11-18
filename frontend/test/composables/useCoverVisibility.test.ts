import { describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

import { useCoverVisibility } from "~/composables/useCoverVisibility";

/**
 * 「这张卡到底有没有封面」（票 440）。
 *
 * 口径必须是**消费方能读到的响应式状态**：没封面要换的是外层排版（栅格列数、卡片高度），
 * 只让封面组件自己清空节点是不够的——那样会留一个空列。
 */
describe("useCoverVisibility", () => {
  it("有地址就有封面", () => {
    const src = ref<string | null>("https://cdn.example.com/a.png");
    const { hasCover, normalizedSrc } = useCoverVisibility(src);

    expect(hasCover.value).toBe(true);
    expect(normalizedSrc.value).toBe("https://cdn.example.com/a.png");
  });

  it("空串 / 只有空白 / null / undefined 都算没有封面", () => {
    for (const value of ["", "   ", null, undefined]) {
      const { hasCover } = useCoverVisibility(ref(value));
      expect(hasCover.value).toBe(false);
    }
  });

  it("图片失败后翻成「没有封面」，消费方据此拿掉整个封面槽", () => {
    const { hasCover, markCoverFailed } = useCoverVisibility(ref("https://cdn.example.com/a.png"));

    expect(hasCover.value).toBe(true);
    markCoverFailed();
    expect(hasCover.value).toBe(false);
  });

  it("换成新地址时失败态复位（上一张的失败不牵连下一张）", async () => {
    const src = ref<string | null>("https://cdn.example.com/a.png");
    const { hasCover, markCoverFailed } = useCoverVisibility(src);

    markCoverFailed();
    expect(hasCover.value).toBe(false);

    src.value = "https://cdn.example.com/b.png";
    await nextTick();
    expect(hasCover.value).toBe(true);
  });

  it("getter 形式也成立（组件里传的是 () => props.cover）", () => {
    const props = ref({ cover: "" });
    const { hasCover } = useCoverVisibility(() => props.value.cover);

    expect(hasCover.value).toBe(false);
    props.value = { cover: "https://cdn.example.com/c.png" };
    expect(hasCover.value).toBe(true);
  });

  it("失败标记是幂等的（连续两次 error 不会翻出别的状态）", () => {
    const { hasCover, markCoverFailed } = useCoverVisibility(ref("https://cdn.example.com/a.png"));
    const spy = vi.fn(markCoverFailed);

    spy();
    spy();
    expect(hasCover.value).toBe(false);
  });
});
