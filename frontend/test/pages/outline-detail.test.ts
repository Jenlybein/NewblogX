import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Suspense, defineComponent, h } from "vue";

import CollectionDetailPage from "~/pages/collections/[id].vue";
import ProjectDetailPage from "~/pages/projects/[id].vue";
import { getCollectionDetail } from "~/services/collection";
import { getProjectDetail } from "~/services/project";
import type { CollectionDetail, ProjectDetail } from "~/types/api";
import type { OutlineNode } from "~/utils/outlineTree";

import { installAwaitableAsyncData } from "../mocks/awaitableAsyncData";

vi.mock("~/services/collection", () => ({ getCollectionDetail: vi.fn() }));
vi.mock("~/services/project", () => ({ getProjectDetail: vi.fn() }));

/**
 * 合集 / 项目详情页的行为接缝（#219 / 决定清单 A8）。
 *
 * 两条路由页只是「同一棵树 + 各自的实体文案」：这里挂载真实页面组件，断言
 * 读者能看到什么——嵌套树、层级、带来源的文章链接、刷新后保留的折叠状态、
 * 空树的中性空态，以及错误（未展示）按不存在处理。`OutlineTree` 的组件细节
 * 由 `test/components/OutlineTree.test.ts` 覆盖；纯映射由 utils 测试覆盖。
 */

const NuxtLinkStub = {
  props: ["to"],
  template: '<a :href="typeof to === \'string\' ? to : String(to)"><slot /></a>',
};

const tree: OutlineNode[] = [
  {
    id: "f1",
    kind: 1,
    title: "第一章",
    children: [
      {
        id: "a1",
        kind: 2,
        article_id: "101",
        article_title: "开篇",
        article_cover: "",
        children: [],
      },
      {
        id: "f2",
        kind: 1,
        title: "深入",
        children: [
          {
            id: "a2",
            kind: 2,
            article_id: "102",
            article_title: "第二篇",
            article_cover: "",
            children: [],
          },
        ],
      },
    ],
  },
  {
    id: "a3",
    kind: 2,
    article_id: "103",
    article_title: "附录",
    children: [],
  },
];

function collectionDetail(nodes: OutlineNode[], articleCount = 3): CollectionDetail {
  return {
    id: "c1",
    title: "Nuxt 部署实战",
    description: "从零到上线",
    cover: "",
    created_at: "2026-01-01T00:00:00Z",
    article_count: articleCount,
    outline: { owner_id: "c1", owner_kind: 1, nodes },
  } as CollectionDetail;
}

function projectDetail(nodes: OutlineNode[], articleCount = 3): ProjectDetail {
  return {
    id: "p1",
    title: "重构实战",
    description: "从零到上线",
    cover: "",
    created_at: "2026-01-01T00:00:00Z",
    article_count: articleCount,
    outline: { owner_id: "p1", owner_kind: 2, nodes },
  } as ProjectDetail;
}

async function mountDetailPage(
  page: typeof CollectionDetailPage | typeof ProjectDetailPage,
  ownerId: string,
) {
  vi.stubGlobal("useRoute", () => ({ params: { id: ownerId }, query: {} }));
  vi.stubGlobal("useSeoMeta", () => {});
  installAwaitableAsyncData();

  // 详情页的 setup 是 async（`await useAsyncData`）：用 Suspense 包住才能渲染，
  // 与 Nuxt 页面在路由器里被 `<Suspense>` 包裹时的行为一致。
  const Host = defineComponent({
    render: () => h(Suspense, null, { default: () => h(page) }),
  });

  const wrapper = mount(Host, {
    global: { stubs: { NuxtLink: NuxtLinkStub } },
  });
  await flushPromises();
  return wrapper;
}

function outlineRowIds(wrapper: ReturnType<typeof mount>): string[] {
  return wrapper.findAll("[data-outline-row]").map((item) => item.attributes("data-outline-row") ?? "");
}

beforeEach(() => {
  window.localStorage.clear();
});

afterEach(() => {
  vi.mocked(getCollectionDetail).mockReset();
  vi.mocked(getProjectDetail).mockReset();
});

describe("合集详情页：目录树", () => {
  it("渲染嵌套树（深度优先 + 层级），点文章节点带 collection:<id> 来源", async () => {
    vi.mocked(getCollectionDetail).mockResolvedValue(collectionDetail(tree));

    const wrapper = await mountDetailPage(CollectionDetailPage, "c1");

    expect(wrapper.get("[data-outline-tree]").exists()).toBe(true);
    expect(outlineRowIds(wrapper)).toEqual(["f1", "a1", "f2", "a2", "a3"]);
    expect(wrapper.findAll("[data-outline-row]").map((item) => item.attributes("data-depth"))).toEqual([
      "0",
      "1",
      "1",
      "2",
      "0",
    ]);
    expect(wrapper.get('[data-outline-row="a1"] a').attributes("href")).toBe(
      "/article/101?from=collection:c1",
    );
    expect(wrapper.get('[data-outline-row="a3"] a').attributes("href")).toBe(
      "/article/103?from=collection:c1",
    );
    // 旧扁平列表的「第 N 篇」位置标记退场：位置只由缩进表达。
    expect(wrapper.text()).not.toMatch(/第\s*\d+\s*篇/u);
    expect(wrapper.findAll("[data-collection-item]")).toHaveLength(0);
  });

  it("篇数读后端算好的公开文章数，不按树里所有节点重算", async () => {
    vi.mocked(getCollectionDetail).mockResolvedValue(collectionDetail(tree, 2));

    const wrapper = await mountDetailPage(CollectionDetailPage, "c1");

    expect(wrapper.text()).toContain("共 2 篇");
  });

  it("空树显示中性空态：不渲染树，也不说破内部状态", async () => {
    vi.mocked(getCollectionDetail).mockResolvedValue(collectionDetail([], 0));

    const wrapper = await mountDetailPage(CollectionDetailPage, "c1");

    expect(wrapper.find("[data-outline-tree]").exists()).toBe(false);
    expect(wrapper.get("[data-empty-state]").text()).toContain("这个合集还没有文章");
    expect(wrapper.text()).not.toMatch(/未公开|未发布|不可见/u);
  });

  it("未展示 / 取数失败按不存在处理：中性空态，不出现「不存在或未公开」", async () => {
    vi.mocked(getCollectionDetail).mockRejectedValue(new Error("not found"));

    const wrapper = await mountDetailPage(CollectionDetailPage, "c1");

    expect(wrapper.find("[data-outline-tree]").exists()).toBe(false);
    expect(wrapper.get("[data-empty-state]").text()).toContain("合集不存在");
    expect(wrapper.text()).not.toMatch(/未公开|未发布|不可见/u);
  });

  it("折叠状态来自 localStorage：已收起的文件夹在刷新后仍收起，点箭头能展开并写回", async () => {
    vi.mocked(getCollectionDetail).mockResolvedValue(collectionDetail(tree));
    window.localStorage.setItem("reader-outline-collapsed:collection:c1", JSON.stringify(["f1"]));

    const wrapper = await mountDetailPage(CollectionDetailPage, "c1");

    expect(outlineRowIds(wrapper)).toEqual(["f1", "a3"]);

    await wrapper.get('[data-outline-toggle="f1"]').trigger("click");
    await flushPromises();

    expect(outlineRowIds(wrapper)).toEqual(["f1", "a1", "f2", "a2", "a3"]);
    expect(window.localStorage.getItem("reader-outline-collapsed:collection:c1")).toBe("[]");
  });
});

describe("项目详情页：目录树", () => {
  it("渲染同一棵树，来源参数换成 project:<id>", async () => {
    vi.mocked(getProjectDetail).mockResolvedValue(projectDetail(tree));

    const wrapper = await mountDetailPage(ProjectDetailPage, "p1");

    expect(wrapper.get("[data-outline-tree]").exists()).toBe(true);
    expect(wrapper.get('[data-outline-row="a2"] a').attributes("href")).toBe(
      "/article/102?from=project:p1",
    );
    expect(wrapper.text()).toContain("共 3 篇");
    expect(wrapper.text()).not.toMatch(/第\s*\d+\s*篇/u);
    expect(wrapper.findAll("[data-project-item]")).toHaveLength(0);
  });

  it("空树显示「这个项目还没有文章」，错误按不存在处理", async () => {
    vi.mocked(getProjectDetail).mockResolvedValue(projectDetail([], 0));

    const wrapper = await mountDetailPage(ProjectDetailPage, "p1");
    expect(wrapper.get("[data-empty-state]").text()).toContain("这个项目还没有文章");
    expect(wrapper.find("[data-outline-tree]").exists()).toBe(false);
  });

  it("未展示的项目按不存在处理：中性空态", async () => {
    vi.mocked(getProjectDetail).mockRejectedValue(new Error("not found"));

    const wrapper = await mountDetailPage(ProjectDetailPage, "p1");
    expect(wrapper.get("[data-empty-state]").text()).toContain("项目不存在");
    expect(wrapper.text()).not.toMatch(/未公开|未发布|不可见/u);
  });

  it("折叠状态按树分开存：项目树不受合集树保存的收起状态影响", async () => {
    vi.mocked(getProjectDetail).mockResolvedValue(projectDetail(tree));
    window.localStorage.setItem("reader-outline-collapsed:collection:c1", JSON.stringify(["f1"]));

    const wrapper = await mountDetailPage(ProjectDetailPage, "p1");

    expect(outlineRowIds(wrapper)).toEqual(["f1", "a1", "f2", "a2", "a3"]);
  });
});
