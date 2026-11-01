import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it, vi } from "vitest";

import StudioWritePublishDialog from "~/components/studio/StudioWritePublishDialog.vue";

// 发布弹窗里"分类旁的新建入口"（#127）。
//
// 这里只测组件对外的表现：登录后入口可见、空白标题点不动创建、
// 合法标题把"去空白后的标题"交给父级、父级说失败时输入框留着让用户改名。
// 真正发请求、并入下拉、自动选中在 useStudioArticle 的用例里验证。

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

  const NSelect = defineComponent({
    name: "NSelect",
    props: { value: { type: [String, Array], default: null }, options: { type: Array, default: () => [] } },
    emits: ["update:value"],
    setup(props) {
      return () => h("div", { "data-test": "select", "data-value": String(props.value ?? "") });
    },
  });

  const NInput = defineComponent({
    name: "NInput",
    props: {
      value: { type: String, default: "" },
      placeholder: { type: String, default: "" },
      disabled: { type: Boolean, default: false },
    },
    emits: ["update:value"],
    setup(props, { emit }) {
      return () =>
        h("input", {
          value: props.value,
          placeholder: props.placeholder,
          disabled: props.disabled,
          onInput: (event: Event) => emit("update:value", (event.target as HTMLInputElement).value),
        });
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

  const NSwitch = defineComponent({
    name: "NSwitch",
    props: { value: { type: Boolean, default: false } },
    emits: ["update:value"],
    setup() {
      return () => h("div", { "data-test": "switch" });
    },
  });

  return { NModal, NCard, NSelect, NInput, NButton, NSwitch };
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
  categoryOptions: [] as { label: string; value: string }[],
  tagOptions: [] as { label: string; value: string }[],
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

function buttons(wrapper: ReturnType<typeof mountDialog>) {
  return wrapper.findAll("button");
}

function buttonByText(wrapper: ReturnType<typeof mountDialog>, text: string) {
  return buttons(wrapper).find((button) => button.text().includes(text));
}

describe("StudioWritePublishDialog · 内联新建分类", () => {
  it("未登录时没有新建分类入口", () => {
    const wrapper = mountDialog({ canCreateCategory: false });

    expect(wrapper.find('input[placeholder="新分类名称"]').exists()).toBe(false);
    expect(buttonByText(wrapper, "新建分类")).toBeUndefined();
  });

  it("登录后分类旁显示新建入口", () => {
    const wrapper = mountDialog();

    expect(buttonByText(wrapper, "新建分类")).toBeTruthy();
  });

  it("空标题与纯空白标题时创建按钮不可点", async () => {
    const wrapper = mountDialog();
    await buttonByText(wrapper, "新建分类")!.trigger("click");

    const input = wrapper.find('input[placeholder="新分类名称"]');
    const create = buttonByText(wrapper, "创建")!;

    expect(create.attributes("disabled")).toBeDefined();

    await input.setValue("   ");
    expect(buttonByText(wrapper, "创建")!.attributes("disabled")).toBeDefined();

    await input.setValue("新分类");
    expect(buttonByText(wrapper, "创建")!.attributes("disabled")).toBeUndefined();
  });

  it("提交时把去掉首尾空白的标题交给父级", async () => {
    const onCreateCategory = vi.fn(async () => true);
    const wrapper = mountDialog({ onCreateCategory });
    await buttonByText(wrapper, "新建分类")!.trigger("click");

    await wrapper.find('input[placeholder="新分类名称"]').setValue("  新分类  ");
    await buttonByText(wrapper, "创建")!.trigger("click");
    await flushPromises();

    expect(onCreateCategory).toHaveBeenCalledTimes(1);
    expect(onCreateCategory).toHaveBeenCalledWith("新分类");
  });

  it("创建成功后面板关闭，回到「新建分类」入口", async () => {
    const wrapper = mountDialog({ onCreateCategory: vi.fn(async () => true) });
    await buttonByText(wrapper, "新建分类")!.trigger("click");
    await wrapper.find('input[placeholder="新分类名称"]').setValue("新分类");
    await buttonByText(wrapper, "创建")!.trigger("click");
    await flushPromises();

    expect(wrapper.find('input[placeholder="新分类名称"]').exists()).toBe(false);
    expect(buttonByText(wrapper, "新建分类")).toBeTruthy();
  });

  it("创建失败（例如重名）时输入框保留，方便用户改名重试", async () => {
    const wrapper = mountDialog({ onCreateCategory: vi.fn(async () => false) });
    await buttonByText(wrapper, "新建分类")!.trigger("click");
    await wrapper.find('input[placeholder="新分类名称"]').setValue("新分类");
    await buttonByText(wrapper, "创建")!.trigger("click");
    await flushPromises();

    expect(wrapper.find('input[placeholder="新分类名称"]').exists()).toBe(true);
    expect((wrapper.find('input[placeholder="新分类名称"]').element as HTMLInputElement).value).toBe("新分类");
  });
});
