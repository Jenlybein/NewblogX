import { mount } from "@vue/test-utils";
import { defineComponent, h, nextTick } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import RelationsPage from "~/pages/studio/relations.vue";

/**
 * 个人中心「关注与粉丝」（票 327）。
 *
 * 关注与粉丝合成一页：页内顶栏切换 关注 / 粉丝 / 互关，列表本体是同一个卡片。
 * 票 327 的后半段把"互关靠客户端筛"换成了**服务端过滤**，所以这一层要钉住：
 *   1. 三个视图都只打一次 `GET /relations`（带对 type），不再有向后扫页；
 *   2. 顶栏徽标来自 `GET /relations/counts`，读失败时只少数字、不影响列表；
 *   3. 关系动作落 follow/unfollow，移除粉丝落 `/relations/fans/{id}`（先二次确认），
 *      两者都按服务端为准重拉列表与计数；
 *   4. 分页按钮按 has_more 禁用/启用。
 */

const message = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }));
const dialog = vi.hoisted(() => ({ warning: vi.fn() }));
const followService = vi.hoisted(() => ({
  followUser: vi.fn(),
  unfollowUser: vi.fn(),
  removeFan: vi.fn(),
  getRelations: vi.fn(),
  getRelationCounts: vi.fn(),
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
    useDialog: () => dialog,
  };
});

function relationItem(overrides: Record<string, unknown> = {}) {
  return {
    user_id: "u1",
    nickname: "被关注的人",
    avatar: "",
    abstract: "",
    follow_time: "2026-11-02T10:00:00Z",
    relation: 2,
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
          props: {
            title: { type: String, default: "" },
            description: { type: String, default: "" },
            pending: { type: Boolean, default: false },
          },
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

  followService.getRelations.mockResolvedValue({ list: [relationItem()], has_more: false });
  followService.getRelationCounts.mockResolvedValue({ follow: 12, fans: 7, mutual: 3 });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("关注与粉丝：三个视图", () => {
  it("默认进「关注」，带 type=follow 取数并渲染卡片", async () => {
    const wrapper = mountPage();
    await settle();

    expect(followService.getRelations).toHaveBeenCalledWith({ type: "follow", page: 1, limit: 24 });
    expect(wrapper.findAll(".relation-card")).toHaveLength(1);
    expect(wrapper.get(".relation-card__name").text()).toBe("被关注的人");
    expect(wrapper.get('[data-role="relation-tab-follow"]').classes()).toContain("is-active");
  });

  it("切到「粉丝」换 type 取数，并把视图写进 URL", async () => {
    const wrapper = mountPage();
    await settle();

    await wrapper.get('[data-role="relation-tab-fans"]').trigger("click");
    await settle();

    expect(followService.getRelations).toHaveBeenLastCalledWith({ type: "fans", page: 1, limit: 24 });
    expect(routerReplace).toHaveBeenCalledWith({ query: { tab: "fans" } });
  });

  it("互关由服务端过滤：只打一次带 type=mutual 的请求，不再向后扫页", async () => {
    const wrapper = mountPage();
    await settle();
    followService.getRelations.mockClear();

    await wrapper.get('[data-role="relation-tab-mutual"]').trigger("click");
    await settle();

    // 关键：一次请求。客户端筛互关的旧做法会在这里打出好几次。
    expect(followService.getRelations).toHaveBeenCalledTimes(1);
    expect(followService.getRelations).toHaveBeenCalledWith({ type: "mutual", page: 1, limit: 24 });
  });

  it("顶栏徽标显示三个计数", async () => {
    const wrapper = mountPage();
    await settle();

    expect(followService.getRelationCounts).toHaveBeenCalledTimes(1);
    expect(wrapper.get('[data-role="relation-tab-follow"]').text()).toContain("12");
    expect(wrapper.get('[data-role="relation-tab-fans"]').text()).toContain("7");
    expect(wrapper.get('[data-role="relation-tab-mutual"]').text()).toContain("3");
  });

  it("计数读失败时只少数字，列表照常可用（绝不当成 0）", async () => {
    followService.getRelationCounts.mockRejectedValue(new Error("boom"));

    const wrapper = mountPage();
    await settle();

    expect(wrapper.findAll(".relation-card")).toHaveLength(1);
    expect(wrapper.get('[data-role="relation-tab-follow"]').text().trim()).toBe("关注");
    expect(wrapper.get('[data-role="relation-tab-follow"]').text()).not.toContain("0");
  });
});

describe("关注与粉丝：关系动作", () => {
  it("粉丝视图里点「回关」调 followUser，并按服务端为准重拉列表与计数", async () => {
    followService.getRelations.mockResolvedValue({ list: [relationItem({ relation: 3 })], has_more: false });

    const wrapper = mountPage();
    await settle();
    await wrapper.get('[data-role="relation-tab-fans"]').trigger("click");
    await settle();

    followService.getRelations.mockClear();
    await wrapper.get('[data-role="relation-action-u1"]').trigger("click");
    await settle();

    expect(followService.followUser).toHaveBeenCalledWith("u1");
    expect(message.success).toHaveBeenCalledWith("已回关");
    expect(followService.getRelations).toHaveBeenCalledTimes(1);
    expect(followService.getRelationCounts).toHaveBeenCalledTimes(2);
  });

  it("关注视图里取消关注调 unfollowUser", async () => {
    const wrapper = mountPage();
    await settle();

    wrapper.findComponent({ name: "RelationCard" }).vm.$emit("toggleFollow");
    await settle();

    expect(followService.unfollowUser).toHaveBeenCalledWith("u1");
    expect(message.success).toHaveBeenCalledWith("已取消关注");
  });

  it("动作失败时给可读提示，且不重拉", async () => {
    const wrapper = mountPage();
    await settle();
    const before = followService.getRelations.mock.calls.length;

    followService.unfollowUser.mockRejectedValue(new Error("网络中断"));

    wrapper.findComponent({ name: "RelationCard" }).vm.$emit("toggleFollow");
    await settle();

    expect(message.error).toHaveBeenCalledWith(expect.stringContaining("网络中断"));
    expect(followService.getRelations.mock.calls.length).toBe(before);
  });
});

describe("关注与粉丝：移除粉丝", () => {
  async function mountFansTab() {
    followService.getRelations.mockResolvedValue({ list: [relationItem({ relation: 3 })], has_more: false });
    const wrapper = mountPage();
    await settle();
    await wrapper.get('[data-role="relation-tab-fans"]').trigger("click");
    await settle();
    followService.getRelations.mockClear();
    return wrapper;
  }

  it("先二次确认，讲清楚是单向的、对方不会收到通知", async () => {
    const wrapper = await mountFansTab();

    wrapper.findComponent({ name: "RelationCard" }).vm.$emit("removeFan");
    await settle();

    expect(dialog.warning).toHaveBeenCalledTimes(1);
    const options = dialog.warning.mock.calls[0][0] as { title: string; content: string };
    expect(options.title).toBe("移除粉丝");
    expect(options.content).toContain("不会收到通知");
    // 还没确认，不能已经调了接口。
    expect(followService.removeFan).not.toHaveBeenCalled();
  });

  it("确认后调 /relations/fans 的服务，重拉列表与计数", async () => {
    const wrapper = await mountFansTab();
    followService.removeFan.mockResolvedValue({});

    wrapper.findComponent({ name: "RelationCard" }).vm.$emit("removeFan");
    await settle();

    const options = dialog.warning.mock.calls[0][0] as { onPositiveClick: () => Promise<void> | void };
    await options.onPositiveClick();
    await settle();

    expect(followService.removeFan).toHaveBeenCalledWith("u1");
    expect(message.success).toHaveBeenCalledWith("已移除该粉丝");
    expect(followService.getRelations).toHaveBeenCalledTimes(1);
    expect(followService.getRelationCounts).toHaveBeenCalledTimes(2);
  });

  it("移除失败时给可读提示（后端会回「该用户不是你的粉丝」）", async () => {
    const wrapper = await mountFansTab();
    followService.removeFan.mockRejectedValue(new Error("该用户不是你的粉丝"));

    wrapper.findComponent({ name: "RelationCard" }).vm.$emit("removeFan");
    await settle();
    const options = dialog.warning.mock.calls[0][0] as { onPositiveClick: () => Promise<void> | void };
    await options.onPositiveClick();
    await settle();

    expect(message.error).toHaveBeenCalledWith(expect.stringContaining("该用户不是你的粉丝"));
    expect(message.success).not.toHaveBeenCalled();
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
    followService.getRelations.mockResolvedValue({ list: [relationItem()], has_more: true });

    const wrapper = mountPage();
    await settle();

    const next = wrapper.get('[data-role="relation-next"]');
    expect(next.attributes("disabled")).toBeUndefined();

    await next.trigger("click");
    await settle();

    expect(followService.getRelations).toHaveBeenLastCalledWith({ type: "follow", page: 2, limit: 24 });
  });

  it("没有更多时下一页禁用", async () => {
    const wrapper = mountPage();
    await settle();

    expect(wrapper.get('[data-role="relation-next"]').attributes("disabled")).toBeDefined();
  });

  it("空列表给空态，而不是一片空白", async () => {
    followService.getRelations.mockResolvedValue({ list: [], has_more: false });

    const wrapper = mountPage();
    await settle();

    expect(wrapper.find(".empty-stub").exists()).toBe(true);
    expect(wrapper.find(".empty-stub").text()).toContain("还没有关注任何人");
  });

  it("取数失败时是失败态，不伪装成空列表", async () => {
    followService.getRelations.mockRejectedValue(new Error("boom"));

    const wrapper = mountPage();
    await settle();

    expect(wrapper.find(".empty-stub").text()).toContain("暂时读不到这份关系列表");
    expect(message.error).toHaveBeenCalled();
  });
});
