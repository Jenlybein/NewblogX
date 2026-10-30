import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";

import ProfileCollectionShelf from "~/components/profile/ProfileCollectionShelf.vue";
import type { FavoriteArticleItem, FavoriteFolderItem } from "~/types/api";

const NuxtLinkStub = defineComponent({
  props: { to: { type: [String, Object], default: "" } },
  setup(_props, { slots }) {
    return () => h("a", slots.default?.());
  },
});

const folder = {
  id: "fav-1",
  title: "工程笔记",
  abstract: "部署与排障",
  article_count: 2,
  has_article: true,
  is_default: false,
} as unknown as FavoriteFolderItem;

const article = {
  article_id: "art-1",
  title: "被收藏的文章",
  abstract: "收藏夹里的文章摘要。",
  cover: "",
  user_nickname: "原作者",
  user_avatar: "",
  article_status: 3,
  favorited_at: "2026-10-05T12:30:00Z",
  digg_count: 1,
  favor_count: 2,
  comment_count: 3,
  view_count: 40,
} as unknown as FavoriteArticleItem;

type ShelfProps = {
  folders?: FavoriteFolderItem[];
  activeFolderId?: string;
  articles?: FavoriteArticleItem[];
  pending?: boolean;
  locked?: boolean;
  lockedTitle?: string;
  lockedDescription?: string;
};

function mountShelf(props: ShelfProps = {}) {
  return mount(ProfileCollectionShelf, {
    props: {
      folders: props.folders ?? [],
      activeFolderId: props.activeFolderId ?? "",
      articles: props.articles ?? [],
      pending: props.pending ?? false,
      locked: props.locked ?? false,
      ...(props.lockedTitle === undefined ? {} : { lockedTitle: props.lockedTitle }),
      ...(props.lockedDescription === undefined ? {} : { lockedDescription: props.lockedDescription }),
    },
    global: {
      stubs: {
        NuxtLink: NuxtLinkStub,
        IconThumbUp: true,
        IconHeart: true,
        IconMessageCircle2: true,
        IconEye: true,
      },
    },
  });
}

describe("ProfileCollectionShelf：收藏架与文章卡片复用同一形态", () => {
  it("有收藏文章时渲染 shelf 卡片，并高亮当前收藏夹", () => {
    const wrapper = mountShelf({ folders: [folder], activeFolderId: "fav-1", articles: [article] });

    const entries = wrapper.findAll(".feed-entry--shelf");
    expect(entries).toHaveLength(1);
    expect(entries[0]?.text()).toContain("被收藏的文章");
    expect(wrapper.get(".profile-folder-card").classes()).toContain("is-active");
    expect(wrapper.get(".profile-shelf-heading").text()).toContain("工程笔记");
  });

  it("空收藏夹不塌陷：内容区保留 lg 尺寸的空态占位、指引与下一步入口", () => {
    const wrapper = mountShelf({ folders: [folder], activeFolderId: "fav-1", articles: [] });

    const empty = wrapper.get(".profile-collection-layout__content [data-empty-state='true']");
    expect(empty.classes()).toContain("min-h-[320px]");
    expect(empty.text()).toContain("这个收藏夹还是空的");
    expect(empty.text()).toContain("换一个收藏夹");
    expect(empty.findAll(".empty-state__hints li")).toHaveLength(2);
    expect(wrapper.findAll(".empty-state__actions a").length).toBeGreaterThanOrEqual(2);
  });

  it("没有任何公开收藏夹时：侧栏给出说明，内容区仍有完整空态结构", () => {
    const wrapper = mountShelf({ folders: [], articles: [] });

    expect(wrapper.get(".profile-folder-note").text()).toContain("还没有公开收藏夹");
    const empty = wrapper.get(".profile-collection-layout__content [data-empty-state='true']");
    expect(empty.classes()).toContain("min-h-[320px]");
  });

  it("加载中：占位结构保留，标记为 pending", () => {
    const wrapper = mountShelf({ pending: true });

    const empty = wrapper.get(".profile-collection-layout__content [data-empty-state='true']");
    expect(empty.attributes("data-state")).toBe("pending");
    expect(empty.classes()).toContain("min-h-[320px]");
    expect(empty.text()).toContain("正在读取收藏内容");
  });

  it("收藏架锁定时给出一致的空态（不是空白区）", () => {
    const wrapper = mountShelf({
      locked: true,
      lockedTitle: "收藏夹暂未公开",
      lockedDescription: "这位作者把收藏夹设成了私密状态。",
    });

    const empty = wrapper.get(".profile-collection-layout__content [data-empty-state='true']");
    expect(empty.text()).toContain("收藏夹暂未公开");
    expect(empty.text()).toContain("私密状态");
    expect(empty.classes()).toContain("min-h-[320px]");
  });
});
