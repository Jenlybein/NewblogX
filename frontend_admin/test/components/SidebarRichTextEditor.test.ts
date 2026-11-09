import { mount } from "@vue/test-utils";
import { nextTick } from "vue";
import { afterEach, describe, expect, it } from "vitest";

import SidebarRichTextEditor from "~/components/settings/SidebarRichTextEditor.vue";

/**
 * 富文本卡正文编辑器的「链接」行为（#329）。
 *
 * 用户报的 bug：后台右栏卡片的富文本卡里设的链接，读者站跳去 example.com。
 * 根因在编辑器：链接工具写死了 `execCommand('createLink', …, 'https://example.com')`，
 * 而正文是 contenteditable —— 没有源码视图，运营根本无法把占位地址改掉，
 * 于是**每一条链接都真的指向 example.com**。
 *
 * 这一层用真实挂载把新行为钉死：
 *   1. 点「链接」不写任何地址，先弹地址输入条；
 *   2. 填真实地址后正文里出现的是那个地址（不再是 example.com）；
 *   3. 非法协议就地报错、不落正文；
 *   4. 选中的若是既有链接，改它的 href 而不是再套一层（否则会留一个空 `<a>`）；
 *   5. 取消后正文不变。
 */

function mountEditor(modelValue = "") {
  return mount(SidebarRichTextEditor, { props: { modelValue } });
}

function bodyOf(wrapper: ReturnType<typeof mountEditor>): HTMLElement {
  return wrapper.get('[data-role="rich-text-body"]').element as HTMLElement;
}

/** 在正文里选中一个既有 `<a>` 的文字（模拟"运营点了要改的那条链接"）。 */
function selectAnchorText(body: HTMLElement) {
  const anchor = body.querySelector("a");
  if (!anchor || !anchor.firstChild) throw new Error("正文里没有可选的链接");
  const range = document.createRange();
  range.selectNodeContents(anchor);
  const selection = window.getSelection();
  if (!selection) throw new Error("环境不支持 Selection");
  selection.removeAllRanges();
  selection.addRange(range);
}

/** 把光标放进正文末尾（模拟"运营刚在正文里打字"）。 */
function caretToEnd(body: HTMLElement) {
  const range = document.createRange();
  range.selectNodeContents(body);
  range.collapse(false);
  const selection = window.getSelection();
  if (!selection) throw new Error("环境不支持 Selection");
  selection.removeAllRanges();
  selection.addRange(range);
}

afterEach(() => {
  window.getSelection()?.removeAllRanges();
});

describe("链接工具：先问地址，再把地址写进正文", () => {
  it("点「链接」不写死 example.com，而是弹出地址输入条", async () => {
    const wrapper = mountEditor("");
    caretToEnd(bodyOf(wrapper));

    await wrapper.get('[data-format="link"]').trigger("click");
    await nextTick();

    expect(wrapper.find('[data-role="rich-text-link-panel"]').exists()).toBe(true);
    // 关键回归：正文此刻还是空的——旧实现会立刻塞进一个 example.com 的链接。
    expect(bodyOf(wrapper).innerHTML).not.toContain("example.com");
    expect((wrapper.get('[data-role="rich-text-link-input"]').element as HTMLInputElement).value).toBe("");
  });

  it("填真实地址后，正文里的链接就是那个地址", async () => {
    const wrapper = mountEditor("");
    caretToEnd(bodyOf(wrapper));

    await wrapper.get('[data-format="link"]').trigger("click");
    await nextTick();
    await wrapper.get('[data-role="rich-text-link-input"]').setValue("https://gentlybeing.cn/article/42");
    await wrapper.get('[data-role="rich-text-link-confirm"]').trigger("click");
    await nextTick();

    const anchor = bodyOf(wrapper).querySelector("a");
    expect(anchor?.getAttribute("href")).toBe("https://gentlybeing.cn/article/42");
    // 输入条收起，正文交回给草稿。
    expect(wrapper.find('[data-role="rich-text-link-panel"]').exists()).toBe(false);
    expect(wrapper.emitted("update:modelValue")?.at(-1)?.[0]).toContain("https://gentlybeing.cn/article/42");
  });

  it("裸域名会补成 https，不会生成相对路径", async () => {
    const wrapper = mountEditor("");
    caretToEnd(bodyOf(wrapper));

    await wrapper.get('[data-format="link"]').trigger("click");
    await nextTick();
    await wrapper.get('[data-role="rich-text-link-input"]').setValue("gentlybeing.cn/post");
    await wrapper.get('[data-role="rich-text-link-confirm"]').trigger("click");
    await nextTick();

    expect(bodyOf(wrapper).querySelector("a")?.getAttribute("href")).toBe("https://gentlybeing.cn/post");
  });
});

describe("链接工具：非法地址不落正文", () => {
  it("javascript: 被拒，正文不变，输入条留着让人改", async () => {
    const wrapper = mountEditor("<p>已有正文</p>");
    caretToEnd(bodyOf(wrapper));

    await wrapper.get('[data-format="link"]').trigger("click");
    await nextTick();
    await wrapper.get('[data-role="rich-text-link-input"]').setValue("javascript:alert(1)");
    await wrapper.get('[data-role="rich-text-link-confirm"]').trigger("click");
    await nextTick();

    expect(bodyOf(wrapper).innerHTML).not.toContain("javascript:");
    expect(bodyOf(wrapper).innerHTML).toContain("已有正文");
    expect(wrapper.find('[data-role="rich-text-link-panel"]').exists()).toBe(true);
    expect(wrapper.get('[data-role="rich-text-link-error"]').text()).toContain("http");
  });

  it("空地址被拒", async () => {
    const wrapper = mountEditor("");
    caretToEnd(bodyOf(wrapper));

    await wrapper.get('[data-format="link"]').trigger("click");
    await nextTick();
    await wrapper.get('[data-role="rich-text-link-confirm"]').trigger("click");
    await nextTick();

    expect(wrapper.find('[data-role="rich-text-link-error"]').exists()).toBe(true);
    expect(bodyOf(wrapper).innerHTML).not.toContain("<a");
  });
});

describe("链接工具：改既有链接而不是再套一层", () => {
  it("选中已有链接时回填原地址，确定后改的是它的 href", async () => {
    const existing = '<p><a href="https://old.example.cn/a">旧链接文字</a></p>';
    const wrapper = mountEditor(existing);
    await nextTick();

    selectAnchorText(bodyOf(wrapper));
    await wrapper.get('[data-format="link"]').trigger("click");
    await nextTick();

    const input = wrapper.get('[data-role="rich-text-link-input"]').element as HTMLInputElement;
    expect(input.value).toBe("https://old.example.cn/a");

    await wrapper.get('[data-role="rich-text-link-input"]').setValue("https://new.example.cn/b");
    await wrapper.get('[data-role="rich-text-link-confirm"]').trigger("click");
    await nextTick();

    const anchors = bodyOf(wrapper).querySelectorAll("a");
    // 只能有一条：再套一层会留下一个空的 `<a>`（读者站上是看不见的脏数据）。
    expect(anchors).toHaveLength(1);
    expect(anchors[0]?.getAttribute("href")).toBe("https://new.example.cn/b");
    expect(anchors[0]?.textContent).toBe("旧链接文字");
  });
});

describe("链接工具：取消不改正文", () => {
  it("点取消后正文与打开前一致", async () => {
    const wrapper = mountEditor('<p><a href="https://keep.example.cn/a">保留</a></p>');
    await nextTick();

    selectAnchorText(bodyOf(wrapper));
    await wrapper.get('[data-format="link"]').trigger("click");
    await nextTick();
    await wrapper.get('[data-role="rich-text-link-cancel"]').trigger("click");
    await nextTick();

    expect(bodyOf(wrapper).innerHTML).toContain("https://keep.example.cn/a");
    expect(wrapper.find('[data-role="rich-text-link-panel"]').exists()).toBe(false);
  });
});

describe("工具栏：其余排版入口不变", () => {
  it("仍是五个入口，且没有任何脚本 / iframe 控件", () => {
    const wrapper = mountEditor("");
    const toolbar = wrapper.get('[data-role="rich-text-toolbar"]');

    expect(toolbar.findAll("[data-format]").map((button) => button.attributes("data-format"))).toEqual([
      "h2",
      "bold",
      "italic",
      "list",
      "link",
    ]);
    expect(toolbar.html().toLowerCase()).not.toContain("onclick");
    expect(toolbar.html()).not.toContain("<iframe");
  });
});
