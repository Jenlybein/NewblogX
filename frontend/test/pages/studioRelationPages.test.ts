import { mount } from "@vue/test-utils";
import { Suspense, defineComponent, h, isRef, ref, shallowRef, watch, type Component, type Ref } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import FansPage from "~/pages/studio/fans.vue";
import FollowsPage from "~/pages/studio/follows.vue";

/**
 * 个人中心的粉丝 / 关注列表（#327）。
 *
 * 用户报的 bug：这两页把操作按钮塞进 naive-ui `NListItem` 的 `#suffix` 槽，
 * 而 `.n-list-item__suffix` 是 `flex: 0`，宽度退化成"内容最小可断行宽度" ——
 * "查看主页"是中文、可任意断行，于是整列被压成一字宽、四个字竖排、列表项被撑高。
 *
 * 修法是"复用优先"：两页改用公开主页已在用的 `ProfileRelationList`（头像 + 昵称 +
 * 关系 chip + 简介 + 时间 + 操作），分页留在页面里。所以这层用例要真的跨过
 * 页面 → 共享组件 → 服务这条线，而不是只断言"页面里出现了某个字符串"：
 *   1. 页面渲染的是共享组件的结构（`.profile-relation-list`），naive 的列表结构已经不在；
 *   2. 动作文案的语境：粉丝页是「取消回关」，关注页仍是「取消关注」；
 *   3. 分页按钮按 `has_more` 禁用 / 启用，点下一页按第 2 页重新取数；
 *   4. 点动作按钮真的调到 follow 服务，并按服务端为准重拉列表。
 *
 * 替身与既有页面用例同理：Nuxt 编译期宏（definePageMeta / useSeoMeta / useNoIndexPage）
 * 与自动导入的 useAsyncData 只能桩掉；naive-ui 换成极小实现（NButton 保留 disabled，
 * 列表组件保留 class 以便断言"旧结构不在了"）；两个 service 换成记录器。
 */

const message = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn(), info: vi.fn() }));
const studioService = vi.hoisted(() => ({ getFanUsers: vi.fn(), getFollowUsers: vi.fn() }));
const followService = vi.hoisted(() => ({ followUser: vi.fn(), unfollowUser: vi.fn() }));

vi.mock("naive-ui", async () => {
  const { defineComponent, h } = await import("vue");

  const NButtonStub = defineComponent({
    name: "NButton",
    props: {
      disabled: { type: Boolean, default: false },
      loading: { type: Boolean, default: false },
      quaternary: { type: Boolean, default: false },
      size: { type: String, default: undefined },
      type: { type: String, default: undefined },
      attrType: { type: String, default: undefined },
    },
    setup(props, { slots }) {
      return () => h("button", { disabled: props.disabled || undefined }, slots.default?.());
    },
  });

  const NAvatarStub = defineComponent({
    name: "NAvatar",
    props: { src: { type: String, default: "" }, round: { type: Boolean, default: true }, size: { type: [Number, String], default: undefined } },
    setup(_props, { slots }) {
      return () => h("span", { class: "avatar-stub" }, slots.default?.());
    },
  });

  // 旧结构的替身：修好之后页面不该再渲染它们（下面会断言 .n-list 不存在）。
  const NListStub = defineComponent({
    name: "NList",
    setup(_props, { slots }) {
      return () => h("div", { class: "n-list" }, slots.default?.());
    },
  });
  const NListItemStub = defineComponent({
    name: "NListItem",
    setup(_props, { slots }) {
      return () => h("div", { class: "n-list-item" }, [slots.default?.(), slots.suffix?.()]);
    },
  });
  const NThingStub = defineComponent({
    name: "NThing",
    props: { title: { type: String, default: "" }, description: { type: String, default: "" } },
    setup(props, { slots }) {
      return () => h("div", { class: "n-thing" }, [props.title, props.description, slots.footer?.()]);
    },
  });
  const NTagStub = defineComponent({
    name: "NTag",
    setup(_props, { slots }) {
      return () => h("span", { class: "n-tag" }, slots.default?.());
    },
  });

  return {
    useMessage: () => message,
    NButton: NButtonStub,
    NAvatar: NAvatarStub,
    NList: NListStub,
    NListItem: NListItemStub,
    NThing: NThingStub,
    NTag: NTagStub,
  };
});

vi.mock("~/services/studio", () => studioService);
vi.mock("~/services/follow", () => followService);

/**
 * `useAsyncData` 替身：会**按 watch 源重跑取数函数**。
 *
 * 共享替身的 refresh 只是再 await 同一个 promise（不重取），也不会跟 watch 源联动，
 * 于是"翻到第 2 页按第 2 页取数""写完重拉列表"这两条在页面用例里观察不到 ——
 * 而它们正是这一票要钉的接线。这里把 options.watch 真的接上。
 */
function useAsyncDataWatching(...args: unknown[]) {
  let handler: (() => unknown) | undefined;
  let watchSources: unknown;

  for (const arg of args) {
    if (typeof arg === "function") {
      handler = arg as () => unknown;
      continue;
    }
    if (arg && typeof arg === "object" && "watch" in (arg as Record<string, unknown>)) {
      watchSources = (arg as { watch?: unknown }).watch;
    }
  }

  const data = ref<unknown>(null);
  const error = ref<unknown>(null);
  const pending = ref(true);

  const run = () =>
    Promise.resolve()
      .then(() => (handler ? handler() : null))
      .then((value) => {
        data.value = value;
      })
      .catch((reason) => {
        error.value = reason;
      })
      .finally(() => {
        pending.value = false;
      });

  const initial = run();

  const sources = Array.isArray(watchSources) ? watchSources : watchSources ? [watchSources] : [];
  for (const source of sources) {
    if (isRef(source)) {
      watch(source as Ref, () => void run(), { flush: "sync" });
    }
  }

  return { data, error, pending, status: pending, refresh: run, execute: run, asyncData: initial };
}

/**
 * 页面用 `await useAsyncData(...)`（Nuxt 里这会有异步 setup），单测里必须放进
 * `<Suspense>` 才会渲染 —— 与 HistoryPage 用例同一做法。
 */
function mountPage(page: Component) {
  return mount(
    defineComponent({
      name: "PageUnderSuspense",
      setup() {
        return () => h(Suspense, null, { default: () => h(page) });
      },
    }),
    {
      global: {
        stubs: {
          NuxtLink: defineComponent({
            props: { to: { type: [String, Object], default: "" } },
            setup(_props, { slots }) {
              return () => h("a", slots.default?.());
            },
          }),
        },
      },
    },
  );
}

/** 等 useAsyncData 的微任务链把数据落到视图上。 */
async function settle() {
  for (let i = 0; i < 6; i += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

/** 当前渲染的每张关系卡片的动作按钮文案。 */
function actionLabels(wrapper: ReturnType<typeof mountPage>): string[] {
  return wrapper.findAll(".profile-relation-card__actions button").map((button) => button.text());
}

function paginationButton(wrapper: ReturnType<typeof mountPage>, label: "上一页" | "下一页") {
  const button = wrapper.findAll("button").find((candidate) => candidate.text() === label);
  if (!button) throw new Error(`找不到分页按钮：${label}`);
  return button;
}

const fansRow = {
  fans_user_id: "f1",
  fans_nickname: "粉丝甲",
  fans_avatar: "",
  fans_abstract: "一个关注了你很久的人",
  follow_time: "2026-11-01T10:00:00Z",
  relation: 4,
};

const followedRow = {
  followed_user_id: "g1",
  followed_nickname: "作者乙",
  followed_avatar: "",
  followed_abstract: "写工程笔记的人",
  follow_time: "2026-11-02T10:00:00Z",
  relation: 4,
};

beforeEach(() => {
  vi.clearAllMocks();

  studioService.getFanUsers.mockResolvedValue({ list: [fansRow], has_more: true });
  studioService.getFollowUsers.mockResolvedValue({ list: [followedRow], has_more: true });
  followService.followUser.mockResolvedValue(undefined);
  followService.unfollowUser.mockResolvedValue(undefined);

  vi.stubGlobal("definePageMeta", () => undefined);
  vi.stubGlobal("useSeoMeta", () => undefined);
  vi.stubGlobal("useNoIndexPage", () => undefined);
  vi.stubGlobal("useAsyncData", useAsyncDataWatching);
  // `shallowRef` 在这两个页面里是 Nuxt 自动导入（SFC 没有写 import）：单测环境没有
  // 编译期注入，只能补到 globalThis 上，否则 setup 会 ReferenceError。
  vi.stubGlobal("shallowRef", shallowRef);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("粉丝页：换用共享关系列表", () => {
  it("渲染 ProfileRelationList（粉丝语境），旧 naive 列表结构不再出现", async () => {
    const wrapper = mountPage(FansPage);
    await settle();

    expect(wrapper.find(".profile-relation-list").exists()).toBe(true);
    expect(wrapper.find(".n-list").exists()).toBe(false);
    expect(wrapper.text()).toContain("粉丝甲");
    // relation === 4：粉丝语境的按钮撤掉的是"你回关的那一半"。
    expect(actionLabels(wrapper)).toEqual(["取消回关"]);
  });

  it("点动作按钮：取消回关走 unfollowUser，并按服务端为准重拉列表", async () => {
    const wrapper = mountPage(FansPage);
    await settle();

    await wrapper.get(".profile-relation-card__actions button").trigger("click");
    await settle();

    expect(followService.unfollowUser).toHaveBeenCalledWith("f1");
    expect(message.success).toHaveBeenCalledWith("已取消回关");
    // 首屏一次 + 操作后一次：写完不本地拼条目，重拉列表。
    expect(studioService.getFanUsers).toHaveBeenCalledTimes(2);
  });

  it("粉丝语境的边界：单向已关注（relation 2）的按钮与提示都是「取消关注」，不是「取消回关」", async () => {
    studioService.getFanUsers.mockResolvedValue({ list: [{ ...fansRow, relation: 2 }], has_more: false });
    const wrapper = mountPage(FansPage);
    await settle();

    expect(actionLabels(wrapper)).toEqual(["取消关注"]);

    await wrapper.get(".profile-relation-card__actions button").trigger("click");
    await settle();

    expect(followService.unfollowUser).toHaveBeenCalledWith("f1");
    // 提示要跟着真正撤掉的那个方向说，否则用户看到"取消回关成功"却找不到回关。
    expect(message.success).toHaveBeenCalledWith("已取消关注");
  });

  it("分页按钮按 has_more 禁用/启用，点下一页按第 2 页取数", async () => {
    const wrapper = mountPage(FansPage);
    await settle();

    expect(paginationButton(wrapper, "上一页").attributes("disabled")).toBeDefined();
    expect(paginationButton(wrapper, "下一页").attributes("disabled")).toBeUndefined();

    await paginationButton(wrapper, "下一页").trigger("click");
    await settle();

    expect(studioService.getFanUsers).toHaveBeenLastCalledWith({ page: 2, limit: 30 });

    // 服务端说没有下一页时（重新挂载的一页），「下一页」必须禁用。
    studioService.getFanUsers.mockResolvedValue({ list: [fansRow], has_more: false });
    const lastPage = mountPage(FansPage);
    await settle();

    expect(paginationButton(lastPage, "下一页").attributes("disabled")).toBeDefined();
  });
});

describe("关注页：换用共享关系列表（关系语境文案不变）", () => {
  it("渲染 ProfileRelationList；按钮仍是关系语境（互相关注 → 取消关注），没有旧列表结构", async () => {
    const wrapper = mountPage(FollowsPage);
    await settle();

    expect(wrapper.find(".profile-relation-list").exists()).toBe(true);
    expect(wrapper.find(".n-list").exists()).toBe(false);
    expect(wrapper.text()).toContain("作者乙");
    expect(actionLabels(wrapper)).toEqual(["取消关注"]);
  });

  it("分页按钮按 has_more 禁用/启用，点下一页按第 2 页取数", async () => {
    const wrapper = mountPage(FollowsPage);
    await settle();

    expect(paginationButton(wrapper, "上一页").attributes("disabled")).toBeDefined();
    expect(paginationButton(wrapper, "下一页").attributes("disabled")).toBeUndefined();

    await paginationButton(wrapper, "下一页").trigger("click");
    await settle();

    expect(studioService.getFollowUsers).toHaveBeenLastCalledWith({ page: 2, limit: 30 });

    studioService.getFollowUsers.mockResolvedValue({ list: [followedRow], has_more: false });
    const lastPage = mountPage(FollowsPage);
    await settle();

    expect(paginationButton(lastPage, "下一页").attributes("disabled")).toBeDefined();
  });

  it("点取消关注走 unfollowUser 并重拉列表", async () => {
    const wrapper = mountPage(FollowsPage);
    await settle();

    await wrapper.get(".profile-relation-card__actions button").trigger("click");
    await settle();

    expect(followService.unfollowUser).toHaveBeenCalledWith("g1");
    expect(message.success).toHaveBeenCalledWith("已取消关注");
    expect(studioService.getFollowUsers).toHaveBeenCalledTimes(2);
  });
});
