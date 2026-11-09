import { mount } from "@vue/test-utils";
import { defineComponent, h, nextTick } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import RelationsPage from "~/pages/studio/relations.vue";

/**
 * 个人中心「关注与粉丝」（票 327）。
 *
 * 关注与粉丝原来是两个页面，现在合成一页：页内顶栏切换 关注 / 粉丝 / 互关，
 * 列表本体是同一个卡片。这一层要真的跨过 页面 → 服务 → 卡片 这条线：
 *   1. 三个视图各自打对接口；切视图会重取、回到第一页、并且写进 URL；
 *   2. 「互关」不是独立接口，而是关注列表按 relation===4 筛出来的——这条必须
 *      有用例，否则以后有人把它改成打一个不存在的接口也不会红；
 *   3. 关系动作落到 follow/unfollow，并按服务端为准重拉；
 *   4. 私信走与作者主页同一份 draft 契约；
 *   5. 分页按钮按 has_more 禁用/启用。
 */

const message = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }));
const followService = vi.hoisted(() => ({
  followUser: vi.fn(),
  unfollowUser: vi.fn(),
  getFollowList: vi.fn(),
  getFansList: vi.fn(),
}));
const draftJump = vi.hoisted(() => ({ jumpToDraftChat: vi.fn() }));
const routerReplace = vi.hoisted(() => vi.fn());

vi.mock("~/services/follow", () => followService);
vi.mock("~/composables/useDraftChatJump", () => ({ useDraftChatJump: () => draftJump }));

vi.mock("naive-ui", async () => {
  const { defineComponent, h } = await import("vue");

  return {
    NButton: defineComponent({
      name: "NButton",
      props: { disabled: { type: Boolean, default: false }, quaternary: { type: Boolean, default: false } },
      setup(props, { slots }) {
        return () => h("button", { disabled: props.disabled || undefined }, slots.default?.());
      },
    }),
    // 卡片里的「更多」菜单：本页用例不点它（那是卡片自己的用例），但要有实现。
    NDropdown: defineComponent({
      name: "NDropdown",
      props: { options: { type: Array, default: () => [] } },
      setup(_props, { slots }) {
        return () => h("div", { class: "dropdown-stub" }, slots.default?.());
      },
    }),
    useMessage: () => message,
  };
});

function followRow(overrides: Record<string, unknown> = {}) {
  return {
    followed_user_id: "u1",
    followed_nickname: "被关注的人",
    followed_avatar: "",
    followed_abstract: "",
    follow_time: "2026-11-02T10:00:00Z",
    relation: 2,
    ...overrides,
  };
}

function fanRow(overrides: Record<string, unknown> = {}) {
  return {
    fans_user_id: "f1",
    fans_nickname: "粉丝甲",
    fans_avatar: "",
    fans_abstract: "",
    follow_time: "2026-11-01T10:00:00Z",
    relation: 3,
    ...overrides,
  };
}

function mountPage() {
  return mount(RelationsPage, {
    global: {
      stubs: {
        NuxtLink: defineComponent({
          props: { to: { type: [String, Object], default: "" } },
          setup(_props, { slots }) {
            return () => h("a", slots.default?.());
          },
        }),
        AppAvatar: defineComponent({ name: "AppAvatar", setup: () => () => h("div", { class: "avatar-stub" }) }),
        ProfileEmptyState: defineComponent({
          name: "ProfileEmptyState",
          props: { title: { type: String, default: "" }, description: { type: String, default: "" }, pending: { type: Boolean, default: false } },
          setup(props) {
            return () => h("div", { class: "empty-stub", "data-pending": props.pending }, [props.title, props.description]);
          },
        }),
      },
    },
  });
}

async function settle() {
  for (let i = 0; i < 6; i += 1) await nextTick();
}

beforeEach(() => {
  vi.clearAllMocks();
  routerReplace.mockResolvedValue(undefined);
  vi.stubGlobal("definePageMeta", () => undefined);
  vi.stubGlobal("useSeoMeta", () => undefined);
  vi.stubGlobal("useNoIndexPage", () => undefined);
  vi.stubGlobal("useRoute", () => ({ query: {}, path: "/studio/relations" }));
  vi.stubGlobal("useRouter", () => ({ replace: routerReplace }));

  followService.getFollowList.mockResolvedValue({ list: [followRow()], has_more: false });
  followService.getFansList.mockResolvedValue({ list: [fanRow()], has_more: false });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("关注与粉丝：三个视图", () => {
  it("默认进「关注」，打关注接口并渲染卡片", async () => {
    const wrapper = mountPage();
    await settle();

    expect(followService.getFollowList).toHaveBeenCalledWith({ page: 1, limit: 24 });
    expect(wrapper.findAll(".relation-card")).toHaveLength(1);
    expect(wrapper.get(".relation-card__name").text()).toBe("被关注的人");
    expect(wrapper.get('[data-role="relation-tab-follow"]').classes()).toContain("is-active");
  });

  it("切到「粉丝」打粉丝接口，并把视图写进 URL", async () => {
    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="relation-tab-fans"]').trigger("click");
    await settle();

    expect(followService.getFansList).toHaveBeenCalledWith({ page: 1, limit: 24 });
    expect(wrapper.get(".relation-card__name").text()).toBe("粉丝甲");
    expect(routerReplace).toHaveBeenCalledWith({ query: { tab: "fans" } });
  });

  it("「互关」不是独立接口：由关注列表筛 relation===4 得来", async () => {
    followService.getFollowList.mockResolvedValue({
      list: [
        followRow({ followed_user_id: "a", followed_nickname: "互关的人", relation: 4 }),
        followRow({ followed_user_id: "b", followed_nickname: "单向关注", relation: 2 }),
      ],
      has_more: false,
    });

    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="relation-tab-mutual"]').trigger("click");
    await settle();

    // 打的是关注接口，不是某个不存在的 /follow/mutual。
    expect(followService.getFollowList).toHaveBeenCalled();
    expect(followService.getFansList).not.toHaveBeenCalled();
    expect(wrapper.findAll(".relation-card")).toHaveLength(1);
    expect(wrapper.get(".relation-card__name").text()).toBe("互关的人");
    expect(wrapper.get(".relation-card__action").text()).toBe("已互粉");
  });

  it("互关视图会继续往后扫关注页，直到凑够一页或没有更多", async () => {
    followService.getFollowList.mockImplementation(async (params: { page: number }) => {
      if (params.page === 1) {
        return { list: [followRow({ followed_user_id: "a", relation: 4 })], has_more: true };
      }
      return { list: [followRow({ followed_user_id: "b", relation: 2 })], has_more: false };
    });

    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="relation-tab-mutual"]').trigger("click");
    await settle();

    // 1 次是进入「关注」时的首屏取数；切到「互关」后再扫 2 个关注页。
    expect(followService.getFollowList).toHaveBeenCalledTimes(3);
    expect(wrapper.findAll(".relation-card")).toHaveLength(1);
  });
});

describe("关注与粉丝：关系动作", () => {
  it("粉丝视图里点「回关」调 followUser，并按服务端为准重拉", async () => {
    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="relation-tab-fans"]').trigger("click");
    await settle();

    // 粉丝视图里 relation=3 → 主按钮是可点的「回关」。
    await wrapper.get('[data-role="relation-action-f1"]').trigger("click");
    await settle();

    expect(followService.followUser).toHaveBeenCalledWith("f1");
    expect(message.success).toHaveBeenCalledWith("已回关");
    expect(followService.getFansList).toHaveBeenCalledTimes(2);
  });

  it("关注视图里取消关注调 unfollowUser（已关注是状态，取关走卡片菜单）", async () => {
    const wrapper = mountPage();
    await settle();

    // relation=2 的主按钮是「已关注」状态（不可点），取消关注由卡片的更多菜单触发。
    wrapper.findComponent({ name: "RelationCard" }).vm.$emit("toggleFollow");
    await settle();

    expect(followService.unfollowUser).toHaveBeenCalledWith("u1");
    expect(message.success).toHaveBeenCalledWith("已取消关注");
  });

  it("动作失败时给可读提示，且不重拉", async () => {
    const wrapper = mountPage();
    await settle();
    const before = followService.getFollowList.mock.calls.length;

    followService.unfollowUser.mockRejectedValue(new Error("网络中断"));

    wrapper.findComponent({ name: "RelationCard" }).vm.$emit("toggleFollow");
    await settle();

    expect(message.error).toHaveBeenCalledWith(expect.stringContaining("网络中断"));
    expect(followService.getFollowList.mock.calls.length).toBe(before);
  });
});

describe("关注与粉丝：私信与分页", () => {
  it("私信走与作者主页同一份 draft 契约", async () => {
    const wrapper = mountPage();
    await settle();

    wrapper.findComponent({ name: "RelationCard" }).vm.$emit("message");
    await settle();

    expect(draftJump.jumpToDraftChat).toHaveBeenCalledWith(
      expect.objectContaining({ receiverId: "u1", receiverNickname: "被关注的人" }),
    );
  });

  it("分页按钮按 has_more 禁用/启用，点下一页按第 2 页取数", async () => {
    followService.getFollowList.mockResolvedValue({ list: [followRow()], has_more: true });

    const wrapper = mountPage();
    await settle();

    const next = wrapper.get('[data-role="relation-next"]');
    expect(next.attributes("disabled")).toBeUndefined();

    await next.trigger("click");
    await settle();

    expect(followService.getFollowList).toHaveBeenLastCalledWith({ page: 2, limit: 24 });
  });

  it("没有更多时下一页禁用", async () => {
    const wrapper = mountPage();
    await settle();

    expect(wrapper.get('[data-role="relation-next"]').attributes("disabled")).toBeDefined();
  });

  it("空列表给空态，而不是一片空白", async () => {
    followService.getFollowList.mockResolvedValue({ list: [], has_more: false });

    const wrapper = mountPage();
    await settle();

    expect(wrapper.find(".empty-stub").exists()).toBe(true);
    expect(wrapper.find(".empty-stub").text()).toContain("还没有关注任何人");
  });

  it("取数失败时是失败态，不伪装成空列表", async () => {
    followService.getFollowList.mockRejectedValue(new Error("boom"));

    const wrapper = mountPage();
    await settle();

    expect(wrapper.find(".empty-stub").text()).toContain("暂时读不到这份关系列表");
    expect(message.error).toHaveBeenCalled();
  });
});
