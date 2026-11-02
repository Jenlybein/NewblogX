import { afterEach, describe, expect, it, vi } from "vitest";
import { createApp, nextTick } from "vue";

/**
 * 后台右栏卡片管理器的组件接线用例。
 *
 * 纯函数层已把五类操作与保存负载钉在单测里，这里只验证**接线**：
 * 加载后每张卡（含停用）都渲染出来、控件改的是同一份草稿、
 * 拖动与上移/下移共用重排函数、保存时按当前顺序整表提交并回读。
 *
 * 与后台既有组件用例同形：真挂载 SFC，把 naive-ui 换成一小组桩，
 * 桩把交互转发成原生事件 / `update:value`。
 */

const { getMock, saveMock, messageMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  saveMock: vi.fn(),
  messageMock: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("~/services/admin", () => ({
  getAdminSidebarCards: getMock,
  updateAdminSidebarCards: saveMock,
}));

vi.mock("naive-ui", async () => {
  const { defineComponent, h } = await import("vue");

  const passthrough = (name: string) =>
    defineComponent({
      name,
      inheritAttrs: false,
      setup(_, { slots, attrs }) {
        return () => h("div", { ...attrs, "data-stub": name }, slots.default ? slots.default() : []);
      },
    });

  const buttonStub = defineComponent({
    name: "NButton",
    inheritAttrs: false,
    setup(_, { slots, attrs }) {
      return () => h("button", { type: "button", ...attrs, "data-stub": "NButton" }, slots.default?.());
    },
  });

  const inputStub = defineComponent({
    name: "NInput",
    inheritAttrs: false,
    props: { value: { type: String, default: "" } },
    emits: ["update:value"],
    setup(props, { emit, attrs }) {
      return () =>
        h("input", {
          ...attrs,
          "data-stub": "NInput",
          value: props.value,
          onInput: (event: Event) => emit("update:value", (event.target as HTMLInputElement).value),
        });
    },
  });

  const switchStub = defineComponent({
    name: "NSwitch",
    inheritAttrs: false,
    props: { value: { type: Boolean, default: false } },
    emits: ["update:value"],
    setup(props, { emit, attrs }) {
      return () =>
        h("button", {
          type: "button",
          ...attrs,
          "data-stub": "NSwitch",
          "data-value": String(props.value),
          onClick: () => emit("update:value", !props.value),
        });
    },
  });

  const selectStub = defineComponent({
    name: "NSelect",
    inheritAttrs: false,
    props: {
      value: { type: String, default: "" },
      options: { type: Array as () => Array<{ label: string; value: string }>, default: () => [] },
    },
    emits: ["update:value"],
    setup(props, { emit, attrs }) {
      return () =>
        h(
          "select",
          {
            ...attrs,
            "data-stub": "NSelect",
            value: props.value,
            onChange: (event: Event) =>
              emit("update:value", (event.target as HTMLSelectElement).value),
          },
          props.options.map(option => h("option", { value: option.value }, option.label)),
        );
    },
  });

  const spanStub = (name: string) =>
    defineComponent({
      name,
      inheritAttrs: false,
      setup(_, { slots, attrs }) {
        return () => h("span", { ...attrs, "data-stub": name }, slots.default?.());
      },
    });

  return {
    NAlert: passthrough("NAlert"),
    NButton: buttonStub,
    NCard: passthrough("NCard"),
    NInput: inputStub,
    NSelect: selectStub,
    NSpace: passthrough("NSpace"),
    NSwitch: switchStub,
    NTag: spanStub("NTag"),
    useMessage: () => messageMock,
  };
});

const { default: SidebarCardsEditor } = await import("~/components/settings/SidebarCardsEditor.vue");
const { effectiveSidebarCardTitle } = await import("~/utils/sidebarCardsAdmin");

const mounted: Array<{ app: ReturnType<typeof createApp>; host: HTMLElement }> = [];

/** 服务端一张卡。 */
function row(
  id: string,
  kind: string,
  title: string,
  sortOrder: number,
  enabled: boolean,
  content = "",
) {
  return { id, kind, title, sort_order: sortOrder, enabled, content };
}

/**
 * 内存里的假服务端：GET 返回当前整表，PUT 按后端口径整表替换。
 *
 * 与 `internal/site/app/sidebar.go` 一致：顺序号取提交下标、标题空白回退默认标题、
 * 停用项照旧保存。用来验证"保存后回读"拿到的是服务端口径而不是本地猜测。
 */
function installFakeServer() {
  let rows = [
    row("11", "hot_articles", "热门文章", 0, true),
    row("12", "latest_comments", "", 1, false),
    row("13", "tag_cloud", "标签云", 2, true),
  ];
  getMock.mockImplementation(() => Promise.resolve({ list: rows }));
  saveMock.mockImplementation(
    (payload: { list: Array<{ kind: string; title: string; enabled: boolean; content: string }> }) => {
      rows = payload.list.map((item, index) => ({
        id: String(100 + index),
        kind: item.kind,
        title: item.title.trim() || effectiveSidebarCardTitle(item.kind, ""),
        sort_order: index,
        enabled: item.enabled,
        // 与后端口径一致：只有富文本卡保留正文。
        content: item.kind === "rich_text" ? item.content.trim() : "",
      }));
      return Promise.resolve(undefined);
    },
  );
  return { rows: () => rows };
}

async function mountEditor() {
  const host = document.createElement("div");
  document.body.appendChild(host);
  const app = createApp(SidebarCardsEditor);
  mounted.push({ app, host });
  app.mount(host);
  await vi.waitFor(() => {
    expect(host.querySelectorAll("[data-card-row]").length).toBeGreaterThan(0);
  });
  return host;
}

afterEach(() => {
  for (const { app, host } of mounted.splice(0)) {
    app.unmount();
    host.remove();
  }
});

function rowsOf(host: HTMLElement): HTMLElement[] {
  return [...host.querySelectorAll<HTMLElement>("[data-card-row]")];
}

function kindsOf(host: HTMLElement): string[] {
  return rowsOf(host).map(rowElement => rowElement.dataset.kind ?? "");
}

async function click(host: HTMLElement, selector: string, index = 0) {
  const elements = host.querySelectorAll<HTMLElement>(selector);
  const element = elements[index];
  if (!element) throw new Error(`没有第 ${index} 个 ${selector}`);
  element.click();
  await nextTick();
  await nextTick();
}

async function changeSelect(host: HTMLElement, value: string) {
  const select = host.querySelector<HTMLSelectElement>('[data-role="kind-select"]');
  if (!select) throw new Error("没有种类选择器");
  select.value = value;
  select.dispatchEvent(new Event("change", { bubbles: true }));
  await nextTick();
}

async function typeTitle(host: HTMLElement, index: number, value: string) {
  const input = rowsOf(host)[index]?.querySelector<HTMLInputElement>('[data-role="title"]');
  if (!input) throw new Error(`第 ${index} 行没有标题输入框`);
  input.value = value;
  input.dispatchEvent(new Event("input", { bubbles: true }));
  await nextTick();
}

async function drag(host: HTMLElement, from: number, to: number) {
  const source = rowsOf(host)[from];
  const target = rowsOf(host)[to];
  if (!source || !target) throw new Error("拖拽源或目标不存在");
  source.dispatchEvent(new Event("dragstart", { bubbles: true }));
  target.dispatchEvent(new Event("drop", { bubbles: true }));
  await nextTick();
  await nextTick();
}

describe("右栏卡片管理器：加载与停用展示", () => {
  it("整表（含停用）都渲染出来，停用项有可见标记但不会被删掉", async () => {
    installFakeServer();
    const host = await mountEditor();

    expect(kindsOf(host)).toEqual(["hot_articles", "latest_comments", "tag_cloud"]);
    expect(rowsOf(host).map(element => element.dataset.enabled)).toEqual(["true", "false", "true"]);
    // 停用行保留在原位，并提示读者侧不展示。
    expect(rowsOf(host)[1]!.querySelector('[data-role="disabled-hint"]')?.textContent).toContain(
      "读者侧不展示",
    );
    // 空白标题用默认标题做占位，仍是可编辑输入框。
    expect(
      rowsOf(host)[1]!.querySelector<HTMLInputElement>('[data-role="title"]')?.placeholder,
    ).toBe("最新评论");
    // 每行都有拖拽把手。
    expect(rowsOf(host).every(element => element.getAttribute("draggable") === "true")).toBe(true);
  });
});

describe("右栏卡片管理器：五类操作都改同一份草稿并整表落库", () => {
  it("新增、改标题、启停、拖动、删除之后保存，提交的列表顺序与界面一致", async () => {
    const server = installFakeServer();
    const host = await mountEditor();

    // 新增：选好种类后追加到末尾。
    await changeSelect(host, "tag_cloud");
    await click(host, '[data-role="add"]');
    expect(rowsOf(host)).toHaveLength(4);
    expect(kindsOf(host)[3]).toBe("tag_cloud");

    // 改标题。
    await typeTitle(host, 3, "友情链接");
    expect(rowsOf(host)[3]!.querySelector<HTMLInputElement>('[data-role="title"]')?.value).toBe(
      "友情链接",
    );

    // 启停：点开关把新卡停用。
    await click(host, '[data-role="enabled"]', 3);
    expect(rowsOf(host)[3]!.dataset.enabled).toBe("false");

    // 拖动：第一张拖到最后。
    await drag(host, 0, 3);
    expect(kindsOf(host)).toEqual(["latest_comments", "tag_cloud", "tag_cloud", "hot_articles"]);

    // 删除：删掉原来的标签云（现在在第 2 位）。
    await click(host, '[data-role="remove"]', 1);
    expect(kindsOf(host)).toEqual(["latest_comments", "tag_cloud", "hot_articles"]);

    // 保存：整表提交，顺序即界面顺序；停用项与空标题都照实提交。
    await click(host, '[data-role="save"]');
    await vi.waitFor(() => expect(saveMock).toHaveBeenCalledTimes(1));

    expect(saveMock.mock.calls[0]![0]).toEqual({
      list: [
        { kind: "latest_comments", title: "", enabled: false, content: "" },
        { kind: "tag_cloud", title: "友情链接", enabled: false, content: "" },
        { kind: "hot_articles", title: "热门文章", enabled: true, content: "" },
      ],
    });

    // 保存后回读服务端整表：标题回退、停用项仍在、顺序保持。
    await vi.waitFor(() => expect(getMock).toHaveBeenCalledTimes(2));
    expect(server.rows().map(item => [item.kind, item.sort_order, item.enabled])).toEqual([
      ["latest_comments", 0, false],
      ["tag_cloud", 1, false],
      ["hot_articles", 2, true],
    ]);
    await vi.waitFor(() =>
      expect(rowsOf(host)[0]!.querySelector<HTMLInputElement>('[data-role="title"]')?.value).toBe(
        "最新评论",
      ),
    );
  });

  it("上移/下移按钮与拖动共用重排结果（键盘可达的等价入口）", async () => {
    installFakeServer();
    const host = await mountEditor();

    await click(host, '[data-role="move-down"]', 0);
    expect(kindsOf(host)).toEqual(["latest_comments", "hot_articles", "tag_cloud"]);
    await click(host, '[data-role="move-up"]', 2);
    expect(kindsOf(host)).toEqual(["latest_comments", "tag_cloud", "hot_articles"]);
  });

  it("删除后全部为空时显示空状态，仍可继续新增", async () => {
    installFakeServer();
    const host = await mountEditor();

    for (let index = 0; index < 3; index += 1) {
      await click(host, '[data-role="remove"]');
    }
    expect(host.querySelectorAll("[data-card-row]")).toHaveLength(0);
    expect(host.querySelector('[data-role="empty"]')?.textContent).toContain("还没有卡片");

    await click(host, '[data-role="add"]');
    expect(rowsOf(host)).toHaveLength(1);
  });
});

describe("右栏预览区：与草稿同源、固定宽度、无须保存", () => {
  function previewOf(host: HTMLElement): HTMLElement {
    const preview = host.querySelector<HTMLElement>('[data-role="sidebar-preview"]');
    if (!preview) throw new Error("编辑器里没有预览区");
    return preview;
  }

  function previewKinds(host: HTMLElement): string[] {
    return [...previewOf(host).querySelectorAll<HTMLElement>("[data-preview-card]")].map(
      element => element.dataset.previewCard ?? "",
    );
  }

  function previewTitle(host: HTMLElement, kind: string): string {
    const title = previewOf(host).querySelector(
      `[data-preview-card="${kind}"] [data-role="preview-card-title"]`,
    );
    return title?.textContent?.trim() ?? "";
  }

  it("按首页右栏固定宽度渲染当前草稿，停用卡不出现，且没有占位块", async () => {
    installFakeServer();
    const host = await mountEditor();
    const preview = previewOf(host);

    // 宽度就是首页右栏的固定列宽，不跟随后台视口断点。
    expect(preview.dataset.previewWidth).toBe("310");
    expect(preview.style.width).toBe("310px");
    // 停用的最新评论不渲染；启用且有数据的另外两张在。
    expect(previewKinds(host)).toEqual(["hot_articles", "tag_cloud"]);
    expect(previewTitle(host, "hot_articles")).toBe("热门文章");
    // 预览不做骨架屏 / 占位块。
    expect(preview.querySelector(".n-skeleton, [data-role='skeleton']")).toBeNull();
  });

  it("改标题、启停、新增都即时反映在预览里，保存接口一次都没被调用", async () => {
    installFakeServer();
    const host = await mountEditor();

    // 改标题：预览立刻变，不需要先点保存。
    await typeTitle(host, 0, "大家在读");
    expect(previewTitle(host, "hot_articles")).toBe("大家在读");

    // 启停：停用热门文章后预览里只剩标签云。
    await click(host, '[data-role="enabled"]', 0);
    expect(previewKinds(host)).toEqual(["tag_cloud"]);
    expect(saveMock).not.toHaveBeenCalled();

    // 新增一张卡：草稿里多一张，预览里立刻多一块（示例内容撑起版面）。
    await click(host, '[data-role="add"]');
    expect(previewKinds(host)).toEqual(["tag_cloud", "hot_articles"]);
    expect(saveMock).not.toHaveBeenCalled();
  });
});

/**
 * 富文本卡的后台编辑面：只提供基础排版能力（标题 / 粗斜体 / 列表 / 链接），
 * 不提供脚本、内联事件或 iframe 入口。正文按 HTML 随整表保存提交。
 */
describe("右栏卡片管理器：富文本卡的编辑与保存", () => {
  it("选富文本卡后出现排版工具栏与可编辑区；不提供任何脚本 / iframe 控件", async () => {
    installFakeServer();
    const host = await mountEditor();

    await changeSelect(host, "rich_text");
    await click(host, '[data-role="add"]');

    const rowElement = rowsOf(host)[3]!;
    expect(rowElement.dataset.kind).toBe("rich_text");
    const editor = rowElement.querySelector('[data-role="rich-text-editor"]');
    expect(editor).not.toBeNull();

    const toolbarButtons = [...rowElement.querySelectorAll<HTMLElement>('[data-format]')].map(
      element => element.dataset.format,
    );
    expect(toolbarButtons).toEqual(["h2", "bold", "italic", "list", "link"]);
    // 工具栏里没有脚本 / iframe / 事件属性入口。
    const toolbarHtml = rowElement.querySelector('[data-role="rich-text-toolbar"]')?.innerHTML ?? "";
    expect(toolbarHtml).not.toContain("<script");
    expect(toolbarHtml.toLowerCase()).not.toContain("onclick");
    expect(toolbarHtml).not.toContain("<iframe");

    const editable = rowElement.querySelector<HTMLElement>('[data-role="rich-text-body"]');
    expect(editable?.getAttribute("contenteditable")).toBe("true");
  });

  it("编辑正文后保存，整表负载带上该卡的 content，其它卡为空串", async () => {
    const server = installFakeServer();
    const host = await mountEditor();

    await changeSelect(host, "rich_text");
    await click(host, '[data-role="add"]');
    await typeTitle(host, 3, "站点公告");

    const editable = rowsOf(host)[3]!.querySelector<HTMLElement>('[data-role="rich-text-body"]');
    if (!editable) throw new Error("没有可编辑区");
    editable.innerHTML = "<h2>更新日志</h2><p><strong>重点</strong></p>";
    editable.dispatchEvent(new Event("input", { bubbles: true }));
    await nextTick();

    await click(host, '[data-role="save"]');
    await vi.waitFor(() => expect(saveMock).toHaveBeenCalledTimes(1));

    const payload = saveMock.mock.calls[0]![0] as {
      list: Array<{ kind: string; content: string }>;
    };
    const richItem = payload.list.find(item => item.kind === "rich_text");
    expect(richItem?.content).toContain("<h2>更新日志</h2>");
    expect(payload.list.filter(item => item.kind !== "rich_text").every(item => item.content === "")).toBe(
      true,
    );

    // 保存后回读：服务端把正文带回来，编辑器回填同一段 HTML。
    await vi.waitFor(() => expect(getMock).toHaveBeenCalledTimes(2));
    expect(server.rows().find(item => item.kind === "rich_text")?.content).toContain(
      "<h2>更新日志</h2>",
    );
    await vi.waitFor(() =>
      expect(
        rowsOf(host)[3]!.querySelector<HTMLElement>('[data-role="rich-text-body"]')?.innerHTML,
      ).toContain("<h2>更新日志</h2>"),
    );
  });
});
