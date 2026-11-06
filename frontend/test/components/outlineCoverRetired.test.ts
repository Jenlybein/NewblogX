import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Suspense, defineComponent, h } from "vue";

import ArticleOutlineAside from "~/components/article/ArticleOutlineAside.vue";
import OutlineTree from "~/components/outline/OutlineTree.vue";
import CollectionDetailPage from "~/pages/collections/[id].vue";
import ProjectDetailPage from "~/pages/projects/[id].vue";
import { getCollectionDetail } from "~/services/collection";
import { getProjectDetail } from "~/services/project";
import type { CollectionDetail, ProjectDetail } from "~/types/api";
import type { ArticleOutlineContext } from "~/utils/outlineContext";
import type { OutlineNode } from "~/utils/outlineTree";

/** 与阅读页 / 详情页一致的桥接替身（页面 setup 是 async，需要 Suspense）。 */
import { installAwaitableAsyncData } from "../mocks/awaitableAsyncData";

vi.mock("~/services/collection", () => ({ getCollectionDetail: vi.fn() }));
vi.mock("~/services/project", () => ({ getProjectDetail: vi.fn() }));

/**
 * 封面退场的**跨使用点**证据（票 295）。
 *
 * 目录树被三处共用：`/collections/[id]`、`/projects/[id]`、阅读页左边缘抽屉。票里
 * 只允许改共用组件，所以这里把四个入口都挂起来，断言**同一条外部事实**：文章行是
 * 纯文字——没有 `img`、没有封面容器、没有封面占位（详情页与抽屉一致）。
 *
 * 断言只读渲染结果，不读类名之外的实现：行的文字内容必须恰好是文章标题。
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
        article_cover: "https://img.example/first.png",
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
            article_cover: "https://img.example/second.png",
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

/** 一个使用点里"没有封面"的全部可观察证据。 */
function expectNoCover(wrapper: ReturnType<typeof mount>): void {
  expect(wrapper.findAll("img")).toHaveLength(0);
  expect(wrapper.find(".outline-tree__cover").exists()).toBe(false);
  expect(wrapper.find(".article-cover-default").exists()).toBe(false);
  expect(wrapper.find("[data-outline-cover]").exists()).toBe(false);
}

/** 文章行只渲染标题文字（`textContent` 恰为标签，没有额外空白/占位）。 */
function expectTextOnlyRows(wrapper: ReturnType<typeof mount>): void {
  const rows = wrapper.findAll("[data-outline-kind='article'] a");

  expect(rows.length).toBeGreaterThan(0);
  for (const row of rows) {
    expect(row.element.children).toHaveLength(1);
    expect(row.text()).toBe(row.element.textContent?.trim());
  }
}

function collectionDetail(nodes: OutlineNode[]): CollectionDetail {
  return {
    id: "c1",
    title: "Nuxt 部署实战",
    description: "从零到上线",
    cover: "",
    created_at: "2026-01-01T00:00:00Z",
    article_count: 3,
    outline: { owner_id: "c1", owner_kind: 1, nodes },
  } as CollectionDetail;
}

function projectDetail(nodes: OutlineNode[]): ProjectDetail {
  return {
    id: "p1",
    title: "重构实战",
    description: "从零到上线",
    cover: "",
    created_at: "2026-01-01T00:00:00Z",
    article_count: 3,
    outline: { owner_id: "p1", owner_kind: 2, nodes },
  } as ProjectDetail;
}

function outlineContext(): ArticleOutlineContext {
  return {
    has_tree: true,
    owner_kind: 2,
    owner_id: "p1",
    nodes: tree,
    current: { id: "a1", article_id: "101", article_title: "开篇" },
    previous: null,
    next: { id: "a2", article_id: "102", article_title: "第二篇" },
  };
}

async function mountDetailPage(
  page: typeof CollectionDetailPage | typeof ProjectDetailPage,
  ownerId: string,
) {
  vi.stubGlobal("useRoute", () => ({ params: { id: ownerId }, query: {} }));
  vi.stubGlobal("useSeoMeta", () => {});
  installAwaitableAsyncData();

  const Host = defineComponent({
    render: () => h(Suspense, null, { default: () => h(page) }),
  });

  const wrapper = mount(Host, { global: { stubs: { NuxtLink: NuxtLinkStub } } });
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  window.localStorage.clear();
});

describe("目录树封面退场：三个使用点一致", () => {
  it("共享组件本身：文章行是纯文字", () => {
    const wrapper = mount(OutlineTree, {
      props: {
        nodes: tree,
        kind: "collection",
        ownerId: "c1",
        collapsed: new Set<string>(),
        currentArticleId: "101",
      },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    expectNoCover(wrapper);
    expectTextOnlyRows(wrapper);
  });

  it("阅读页左边缘抽屉：同一棵树，也没有封面", () => {
    const wrapper = mount(ArticleOutlineAside, {
      props: { context: outlineContext(), pending: false, currentArticleId: "101" },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });

    expect(wrapper.find("[data-outline-tree]").exists()).toBe(true);
    expectNoCover(wrapper);
    expectTextOnlyRows(wrapper);
  });

  it("合集详情页：不再渲染封面", async () => {
    vi.mocked(getCollectionDetail).mockResolvedValue(collectionDetail(tree));

    const wrapper = await mountDetailPage(CollectionDetailPage, "c1");

    expect(wrapper.find("[data-outline-tree]").exists()).toBe(true);
    expectNoCover(wrapper);
    expectTextOnlyRows(wrapper);
  });

  it("项目详情页：不再渲染封面", async () => {
    vi.mocked(getProjectDetail).mockResolvedValue(projectDetail(tree));

    const wrapper = await mountDetailPage(ProjectDetailPage, "p1");

    expect(wrapper.find("[data-outline-tree]").exists()).toBe(true);
    expectNoCover(wrapper);
    expectTextOnlyRows(wrapper);
  });
});
