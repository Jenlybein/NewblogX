import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it, vi } from "vitest";

import StudioWritePublishDialog from "~/components/studio/StudioWritePublishDialog.vue";

/**
 * 发布弹窗的字段可访问性（#435 / #423 第 22 项）。
 *
 * 验收点（只钉 DOM 契约，不钉样式）：
 *   - 五个有控件的字段，label 的 `for` 指向一个真实存在的元素 id（点 label 会落到控件上）；
 *   - 「评论开关」不是 labelable 元素，用可读名称（aria-label）承担字段名；
 *   - 内联新建分类与摘要输入同样有字段名，而不是只靠 placeholder 说话。
 *
 * 这里不复用 StudioWritePublishDialog.test.ts 的 naive-ui 替身：那一份的 NInput
 * 不接收 `input-props`，也没有 role=switch 的开关，无法验证 id 落到真实控件上。
 */

vi.mock("naive-ui", () => {
  const NModal = defineComponent({
    name: "NModal",
    props: { show: { type: Boolean, default: false } },
    setup(props, { slots }) {
      return () => (props.show ? h("div", { "data-test": "modal" }, slots.default?.()) : null);
    },
  });

  const NCard = defineComponent({
    name: "NCard",
    setup(_, { slots }) {
      return () => h("section", { "data-test": "card" }, [slots.default?.(), slots.footer?.()]);
    },
  });

  const NButton = defineComponent({
    name: "NButton",
    props: {
      disabled: { type: Boolean, default: false },
      loading: { type: Boolean, default: false },
    },
    emits: ["click"],
    setup(props, { slots, emit }) {
      return () =>
        h("button", { disabled: props.disabled, onClick: () => emit("click") }, slots.default?.());
    },
  });

  // NInput 把 input-props 原样摊到内层 input / textarea 上，才能断言 label 的 for 真的命中控件。
  const NInput = defineComponent({
    name: "NInput",
    inheritAttrs: false,
    props: {
      value: { type: String, default: "" },
      type: { type: String, default: "text" },
      placeholder: { type: String, default: "" },
      inputProps: { type: Object, default: () => ({}) },
    },
    emits: ["update:value"],
    setup(props, { attrs, emit }) {
      return () =>
        h(
          props.type === "textarea" ? "textarea" : "input",
          {
            ...attrs,
            ...props.inputProps,
            value: props.value,
            placeholder: props.placeholder,
            onInput: (event: Event) => emit("update:value", (event.target as HTMLInputElement).value),
          },
        );
    },
  });

  const NSwitch = defineComponent({
    name: "NSwitch",
    inheritAttrs: false,
    props: { value: { type: Boolean, default: false } },
    emits: ["update:value"],
    setup(_, { attrs }) {
      return () => h("div", { ...attrs, role: "switch" });
    },
  });

  const NAlert = defineComponent({
    name: "NAlert",
    setup(_, { slots }) {
      return () => h("div", { "data-test": "alert" }, [slots.header?.(), slots.default?.()]);
    },
  });

  const NPopover = defineComponent({
    name: "NPopover",
    setup(_, { slots }) {
      return () => h("div", { "data-test": "popover" }, [slots.trigger?.(), slots.default?.()]);
    },
  });

  const NTag = defineComponent({
    name: "NTag",
    setup(_, { slots }) {
      return () => h("span", { "data-test": "tag" }, slots.default?.());
    },
  });

  const NSelect = defineComponent({
    name: "NSelect",
    setup() {
      return () => h("div", { "data-test": "select" });
    },
  });

  return { NModal, NCard, NButton, NInput, NSwitch, NAlert, NPopover, NTag, NSelect };
});

const baseProps = {
  show: true,
  categoryId: null as string | null,
  tagIds: [] as string[],
  abstractText: "",
  commentsToggle: true,
  visibility: "visible" as "visible" | "user_hidden",
  locked: false,
  draftPending: false,
  publishPending: false,
  aiPending: false,
  coverUploading: false,
  coverPreviewUrl: "",
  coverStage: "",
  coverError: "",
  coverSize: null,
  categoryOptions: [] as { label: string; value: string }[],
  categoryError: "",
  tagOptions: [] as { label: string; value: string }[],
  tagError: "",
  visibilityOptions: [] as { label: string; value: string }[],
  draftLabel: "保存草稿",
  publishLabel: "确定并发布",
  draftDisabled: false,
  publishDisabled: false,
  onCoverChange: () => {},
  onClear: () => {},
  canCreateCategory: true,
  categoryCreating: false,
  onCreateCategory: vi.fn(async () => true),
};

function mountDialog(overrides: Partial<typeof baseProps> = {}) {
  return mount(StudioWritePublishDialog, { props: { ...baseProps, ...overrides } });
}

/** 每个 label 的 `for` 都要指向弹窗里真实存在的元素。 */
function expectLabelTargets(wrapper: ReturnType<typeof mountDialog>) {
  const labels = wrapper.findAll("label.publish-form__label");

  expect(labels.length).toBeGreaterThan(0);
  for (const label of labels) {
    const target = label.attributes("for");
    expect(target, `label「${label.text()}」缺少 for`).toBeTruthy();
    expect(wrapper.find(`#${target}`).exists(), `label「${label.text()}」指向不存在的 #${target}`).toBe(true);
  }
  return labels;
}

describe("发布弹窗 · 字段 label 与控件的关联", () => {
  it("分类 / 标签 / 封面 / 摘要 / 可见范围五个 label 都指向真实控件", () => {
    const wrapper = mountDialog();
    const labels = expectLabelTargets(wrapper);

    expect(labels.map((label) => label.text())).toEqual([
      "分类",
      "添加标签",
      "文章封面",
      "编辑摘要",
      "可见范围",
    ]);

    expect(wrapper.get("#publish-category-control").attributes("aria-haspopup")).toBe("listbox");
    expect(wrapper.get("#publish-tags-control").attributes("aria-haspopup")).toBe("listbox");
    expect(wrapper.get("#publish-cover-control").element.tagName.toLowerCase()).toBe("button");
    expect(wrapper.get("#publish-abstract-control").element.tagName.toLowerCase()).toBe("textarea");
    expect(wrapper.get("#publish-visibility-control").attributes("aria-haspopup")).toBe("listbox");

    // 分类 label 点击落到分类下拉的触发按钮上（而不是一个没有控件的装饰文字）。
    expect(wrapper.get('label[for="publish-category-control"]').text()).toBe("分类");
  });

  it("评论开关有可读名称（不是只有旁边一句状态文案）", () => {
    const wrapper = mountDialog();

    const toggle = wrapper.get('[role="switch"]');
    expect(toggle.attributes("aria-label")).toBe("评论开关");
  });

  it("内联新建分类的输入框有字段名，不只靠 placeholder", async () => {
    const wrapper = mountDialog();
    await wrapper.findAll("button").find((button) => button.text().includes("新建分类"))!.trigger("click");

    const input = wrapper.get("#publish-new-category-control");
    const label = wrapper.get('label[for="publish-new-category-control"]');
    expect(label.text()).toBe("新分类名称");
    expect(input.attributes("placeholder")).toBe("新分类名称");
  });
});
