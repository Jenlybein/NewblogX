import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import StudioSidebarNav from "~/components/studio/StudioSidebarNav.vue";

/**
 * 个人中心侧栏（#327）。
 *
 * 「分类管理」从公开主页搬进个人中心后，侧栏必须给出入口 —— 否则用户只能靠记 URL。
 * 位置按"内容相关的东西挨着"：收藏夹之后、关注之前。
 *
 * 断言导航项的**顺序与目标**，而不是"源码里出现了某个字符串"：这一层是用户真正
 * 看见并点击的东西。
 */

const authStore = vi.hoisted(() => ({
  profileId: "100" as string | null,
  profileName: "作者甲",
  profileAvatar: "",
  currentUser: { abstract: "" },
}));

function mountNav() {
  return mount(StudioSidebarNav, {
    global: {
      stubs: {
        NuxtLink: defineComponent({
          props: { to: { type: [String, Object], default: "" } },
          setup(props, { slots }) {
            return () => h("a", { "data-to": String(props.to) }, slots.default?.());
          },
        }),
        // 头像走 naive-ui，这里只关心导航，桩掉省事。
        AppAvatar: defineComponent({
          setup() {
            return () => h("span", { class: "avatar-stub" });
          },
        }),
      },
    },
  });
}

function navEntries(wrapper: ReturnType<typeof mountNav>) {
  return wrapper.findAll(".studio-sidebar__link").map((link) => ({
    label: link.text().trim(),
    to: link.attributes("data-to"),
  }));
}

beforeEach(() => {
  vi.stubGlobal("useRoute", () => ({ path: "/studio/dashboard" }));
  vi.stubGlobal("useAuthStore", () => authStore);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("个人中心侧栏：分类入口与关注粉丝合并", () => {
  it("有「分类」项，指向 /studio/categories", () => {
    const wrapper = mountNav();

    const categories = navEntries(wrapper).find((entry) => entry.label === "分类");

    expect(categories).toBeDefined();
    expect(categories?.to).toBe("/studio/categories");
  });

  it("「分类」紧跟在「收藏夹」之后（内容相关的东西挨着）", () => {
    const labels = navEntries(mountNav()).map((entry) => entry.label);

    expect(labels.indexOf("分类")).toBe(labels.indexOf("收藏夹") + 1);
  });

  it("关注与粉丝合成一项「关注与粉丝」，指向 /studio/relations", () => {
    const entries = navEntries(mountNav());

    // 旧的两个入口必须退场：留着它们等于同一份数据有两个入口、两套实现。
    expect(entries.some((entry) => entry.label === "关注")).toBe(false);
    expect(entries.some((entry) => entry.label === "粉丝")).toBe(false);

    const relations = entries.find((entry) => entry.label === "关注与粉丝");
    expect(relations?.to).toBe("/studio/relations");
  });

  it("其余入口保持原样（没有顺手改动既有导航）", () => {
    const labels = navEntries(mountNav()).map((entry) => entry.label);

    expect(labels).toEqual(["数据概览", "我的文章", "收藏夹", "分类", "关注与粉丝", "登录设备", "账号设置"]);
  });
});
