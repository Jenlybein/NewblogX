import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

import FavoriteFolderModal from "~/components/favorite/FavoriteFolderModal.vue";
import type { FavoriteFolderItem } from "~/types/api";

/**
 * 收藏夹弹窗的「已收藏 → 移除」分支（票 #235 第 5 项 / ADR-0024 决策 6）。
 *
 * 既有缺陷：按钮文案是"移出收藏夹"，但请求走 `POST /articles/favorite`
 * → 后端 `Favorite` 发现关系已存在，直接返回 `ErrItemExists`
 * （"已收藏该文章"）。用户点"移除"反而得到一条失败提示，且关系没动。
 *
 * 这里只测组件对外的协议：移除分支发 DELETE（不是 POST），并且把写响应里的
 * 权威 `favor_count` 透传给父级（页面据此 patch，不再重新取正文）。
 */
const mocks = vi.hoisted(() => ({
  getOwnFavoriteFolders: vi.fn(),
  createFavoriteFolder: vi.fn(),
  favoriteArticle: vi.fn(),
  removeFavoriteFolderArticles: vi.fn(),
}));

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
      return () => h("section", { "data-test": "card" }, slots.default?.());
    },
  });
  const NButton = defineComponent({
    name: "NButton",
    props: { disabled: { type: Boolean, default: false }, loading: { type: Boolean, default: false } },
    emits: ["click"],
    setup(props, { slots, emit }) {
      return () => h("button", { disabled: props.disabled, onClick: () => emit("click") }, slots.default?.());
    },
  });
  const NEmpty = defineComponent({
    name: "NEmpty",
    props: { description: { type: String, default: "" } },
    setup(props) {
      return () => h("div", { "data-test": "empty" }, props.description);
    },
  });
  const NInput = defineComponent({
    name: "NInput",
    setup() {
      return () => h("input");
    },
  });
  const NSkeleton = defineComponent({
    name: "NSkeleton",
    setup() {
      return () => h("div", { "data-test": "skeleton" });
    },
  });

  return { NModal, NCard, NButton, NEmpty, NInput, NSkeleton, useMessage: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }) };
});

vi.mock("~/services/favorite", () => ({
  getOwnFavoriteFolders: mocks.getOwnFavoriteFolders,
  createFavoriteFolder: mocks.createFavoriteFolder,
  removeFavoriteFolderArticles: mocks.removeFavoriteFolderArticles,
}));

vi.mock("~/services/article", () => ({
  favoriteArticle: mocks.favoriteArticle,
}));

const ARTICLE_ID = "a1";

function makeFolder(overrides: Partial<FavoriteFolderItem> = {}): FavoriteFolderItem {
  return {
    id: "f1",
    user_id: "u1",
    title: "架构灵感",
    cover: "",
    abstract: "说明",
    is_default: false,
    article_count: 2,
    has_article: true,
    nickname: "",
    avatar: "",
    created_at: "",
    updated_at: "",
    ...overrides,
  } as FavoriteFolderItem;
}

async function mountModal() {
  const wrapper = mount(FavoriteFolderModal, {
    props: { show: false, articleId: ARTICLE_ID, articleTitle: "正文标题" },
  });
  await wrapper.setProps({ show: true });
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  mocks.getOwnFavoriteFolders.mockReset().mockResolvedValue({ count: 1, list: [makeFolder()] });
  mocks.createFavoriteFolder.mockReset();
  mocks.favoriteArticle.mockReset().mockResolvedValue({ is_favor: true, favor_count: 1 });
  mocks.removeFavoriteFolderArticles.mockReset().mockResolvedValue({ is_favor: false, favor_count: 0 });
});

describe("FavoriteFolderModal：已收藏 → 移除", () => {
  it("移除分支走 DELETE（不是再 POST 一次撞 ErrItemExists）", async () => {
    const wrapper = await mountModal();
    const removeButton = wrapper.findAll("button").find((node) => node.text().includes("移出收藏夹"));
    expect(removeButton).toBeTruthy();

    await removeButton!.trigger("click");
    await flushPromises();

    expect(mocks.removeFavoriteFolderArticles).toHaveBeenCalledWith({
      favoriteId: "f1",
      articleIds: [ARTICLE_ID],
    });
    expect(mocks.favoriteArticle).not.toHaveBeenCalled();
  });

  it("移除后把服务端权威 favor_count 交给父级（页面直接 patch，不重新取正文）", async () => {
    const wrapper = await mountModal();
    const removeButton = wrapper.findAll("button").find((node) => node.text().includes("移出收藏夹"));

    await removeButton!.trigger("click");
    await flushPromises();

    const updated = wrapper.emitted("updated");
    expect(updated?.at(-1)).toEqual([{ is_favor: false, favor_count: 0 }]);
    // 列表也会重取：移除后该收藏夹的 has_article 应变为 false。
    expect(mocks.getOwnFavoriteFolders).toHaveBeenCalledTimes(2);
  });

  it("未收藏分支仍走 POST（不要为了修移除把加入也改坏）", async () => {
    mocks.getOwnFavoriteFolders.mockResolvedValue({ count: 1, list: [makeFolder({ has_article: false })] });
    const wrapper = await mountModal();
    const addButton = wrapper.findAll("button").find((node) => node.text().includes("加入收藏夹"));

    await addButton!.trigger("click");
    await flushPromises();

    expect(mocks.favoriteArticle).toHaveBeenCalledWith(ARTICLE_ID, "f1");
    expect(mocks.removeFavoriteFolderArticles).not.toHaveBeenCalled();
  });
});
