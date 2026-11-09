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

// 分类下拉的空状态与错误提示（#320）。
//
// 这两件事必须长得不一样："一个分类都没有"是正常状态（给一句能照着做的提示），
// "列表没加载出来"是故障（给原因）。以前两者都是一个空白下拉。
describe("StudioWritePublishDialog · 分类下拉的空状态与错误（#320）", () => {
  it("加载失败时就地展示原因，而不是空白下拉", () => {
    const wrapper = mountDialog({ categoryError: "网络断开" });

    const hint = wrapper.find('[data-role="category-error"]');
    expect(hint.exists()).toBe(true);
    expect(hint.text()).toContain("网络断开");
    expect(wrapper.find('[data-role="category-empty"]').exists()).toBe(false);
  });

  it("没有任何分类且能新建时，提示可以就地建一个", () => {
    const wrapper = mountDialog({ categoryOptions: [], canCreateCategory: true });

    const hint = wrapper.find('[data-role="category-empty"]');
    expect(hint.exists()).toBe(true);
    expect(hint.text()).toContain("还没有分类");
    expect(hint.text()).toContain("新建分类");
  });

  it("未登录且没有分类时给一句不可建的空状态（不能引导到看不见的入口）", () => {
    const wrapper = mountDialog({ categoryOptions: [], canCreateCategory: false });

    const hint = wrapper.find('[data-role="category-empty"]');
    expect(hint.exists()).toBe(true);
    expect(hint.text()).not.toContain("新建分类");
  });

  it("有分类可选时不显示空状态", () => {
    const wrapper = mountDialog({ categoryOptions: [{ label: "后端", value: "cat-1" }] });

    expect(wrapper.find('[data-role="category-empty"]').exists()).toBe(false);
    expect(wrapper.find('[data-role="category-error"]').exists()).toBe(false);
  });
});

// 标签下拉的错误提示（#322 / 本批 P1）。
//
// 标签侧原来还是 `.catch(() => [])`：接口挂了和"一个标签都还没建"长得一模一样。
// 现在与分类同款：失败给原因，成功/为空不给错误。
describe("StudioWritePublishDialog · 标签下拉的错误提示", () => {
  it("加载失败时就地展示原因，而不是空的下拉", () => {
    const wrapper = mountDialog({ tagError: "网络断开" });

    const hint = wrapper.find('[data-role="tag-error"]');
    expect(hint.exists()).toBe(true);
    expect(hint.text()).toContain("网络断开");
  });

  it("没有错误时不渲染标签错误提示（空列表是正常状态）", () => {
    const wrapper = mountDialog({ tagOptions: [], tagError: "" });

    expect(wrapper.find('[data-role="tag-error"]').exists()).toBe(false);
  });
});
