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
function row(id: string, kind: string, title: string, sortOrder: number, enabled: boolean) {
  return { id, kind, title, sort_order: sortOrder, enabled };
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
  saveMock.mockImplementation((payload: { list: Array<{ kind: string; title: string; enabled: boolean }> }) => {
    rows = payload.list.map((item, index) => ({
      id: String(100 + index),
      kind: item.kind,
      title: item.title.trim() || effectiveSidebarCardTitle(item.kind, ""),
      sort_order: index,
      enabled: item.enabled,
    }));
    return Promise.resolve(undefined);
  });
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
        { kind: "latest_comments", title: "", enabled: false },
        { kind: "tag_cloud", title: "友情链接", enabled: false },
        { kind: "hot_articles", title: "热门文章", enabled: true },
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
