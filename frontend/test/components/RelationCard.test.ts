import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it, vi } from "vitest";

import RelationCard from "~/components/social/RelationCard.vue";

/**
 * 关系卡片（关注 / 粉丝 / 互关三视图共用，票 327）。
 *
 * 形状参照用户给的截图：长方形容器里圆形头像 + 昵称 + 行内按钮，
 * 右侧「更多」入口里是私信与关系动作。这一层只锁**文案与信号**：
 * 哪种关系显示什么按钮、菜单里有什么、点了抛什么事件——样式不在断言范围。
 *
 * naive-ui 的 `NDropdown` 换成能观察 options 的替身：菜单项是数据不是模板，
 * 直接断言列表比"点开再找文字"更稳。
 */

/** naive-ui 的 `NDropdown` 换成能观察 options 的替身（工厂必须自包含：vi.mock 会被提升）。 */
vi.mock("naive-ui", async () => {
  const { defineComponent, h } = await import("vue");

  const DropdownStub = defineComponent({
    name: "NDropdown",
    props: { options: { type: Array, default: () => [] } },
    emits: ["select"],
    setup(props, { slots, emit }) {
      return () =>
        h("div", { class: "dropdown-stub" }, [
          slots.default?.(),
          h(
            "div",
            { class: "dropdown-stub__options" },
            (props.options as Array<{ key: string; label: string }>).map((option) =>
              h(
                "button",
                {
                  class: "dropdown-stub__option",
                  "data-key": option.key,
                  onClick: () => emit("select", option.key),
                },
                option.label,
              ),
            ),
          ),
        ]);
    },
  });

  return { NDropdown: DropdownStub };
});

function mountCard(props: Record<string, unknown>) {
  return mount(RelationCard, {
    props: { userId: "u1", nickname: "某个用户", ...props },
    global: {
      stubs: {
        NuxtLink: defineComponent({
          props: { to: { type: [String, Object], default: "" } },
          setup(props, { slots }) {
            return () =>
              h(
                "a",
                { "data-href": typeof props.to === "string" ? props.to : JSON.stringify(props.to) },
                slots.default?.(),
              );
          },
        }),
        AppAvatar: defineComponent({
          name: "AppAvatar",
          setup() {
            return () => h("div", { class: "avatar-stub" });
          },
        }),
      },
    },
  });
}

function primaryLabel(wrapper: ReturnType<typeof mountCard>): string {
  return wrapper.get(".relation-card__action").text();
}

function menuLabels(wrapper: ReturnType<typeof mountCard>): string[] {
  return wrapper.findAll(".dropdown-stub__option").map((item) => item.text());
}

describe("关系卡片：按钮文案按关系与视图决定", () => {
  it("粉丝视图里「对方关注了我」显示「回关」（可点）", () => {
    const wrapper = mountCard({ relation: 3, mode: "fans" });

    expect(primaryLabel(wrapper)).toBe("回关");
    expect(wrapper.get(".relation-card__action").attributes("disabled")).toBeUndefined();
  });

  it("互关显示「已互粉」，是状态不是按钮（不可点）", () => {
    const wrapper = mountCard({ relation: 4, mode: "mutual" });

    expect(primaryLabel(wrapper)).toBe("已互粉");
    expect(wrapper.get(".relation-card__action").attributes("disabled")).toBeDefined();
    expect(wrapper.get(".relation-card__action").classes()).toContain("is-state");
  });

  it("关注视图里已关注是状态、互关是状态、未关注是动作", () => {
    expect(primaryLabel(mountCard({ relation: 2, mode: "follow" }))).toBe("已关注");
    expect(primaryLabel(mountCard({ relation: 4, mode: "follow" }))).toBe("已互粉");
    expect(primaryLabel(mountCard({ relation: 0, mode: "follow" }))).toBe("关注");
    expect(primaryLabel(mountCard({ relation: 3, mode: "follow" }))).toBe("回关");
  });
});

describe("关系卡片：更多菜单", () => {
  it("已关注的人：发私信 / 取消关注", () => {
    expect(menuLabels(mountCard({ relation: 4, mode: "follow" }))).toEqual(["发私信", "取消关注"]);
  });

  it("对方关注了我、我还没回：发私信 / 回关", () => {
    expect(menuLabels(mountCard({ relation: 3, mode: "fans" }))).toEqual(["发私信", "回关"]);
  });

  it("没有「移除粉丝」：后端只能取消我自己的关注，不能移除别人的关注", () => {
    for (const mode of ["follow", "fans", "mutual"] as const) {
      expect(menuLabels(mountCard({ relation: 3, mode }))).not.toContain("移除粉丝");
    }
  });

  it("选菜单项抛对应事件", async () => {
    const wrapper = mountCard({ relation: 4, mode: "follow" });

    await wrapper.get('[data-key="message"]').trigger("click");
    expect(wrapper.emitted("message")).toHaveLength(1);

    await wrapper.get('[data-key="toggleFollow"]').trigger("click");
    expect(wrapper.emitted("toggleFollow")).toHaveLength(1);
  });
});

describe("关系卡片：主按钮与头像", () => {
  it("点可点的主按钮抛 toggleFollow；状态按钮点了什么也不抛", async () => {
    const actionable = mountCard({ relation: 3, mode: "fans" });
    await actionable.get(".relation-card__action").trigger("click");
    expect(actionable.emitted("toggleFollow")).toHaveLength(1);

    const stateful = mountCard({ relation: 4, mode: "mutual" });
    await stateful.get(".relation-card__action").trigger("click");
    expect(stateful.emitted("toggleFollow")).toBeUndefined();
  });

  it("头像与昵称都指向对方主页", () => {
    const wrapper = mountCard({ userId: "u9", nickname: "某人" });

    // 两个入口（头像 + 昵称）都挂到对方主页，避免"点名字不会跳"的空点击。
    expect(wrapper.get(".relation-card__avatar").attributes("data-href")).toBe("/users/u9");
    expect(wrapper.get(".relation-card__name").attributes("data-href")).toBe("/users/u9");
    expect(wrapper.get(".relation-card__name").text()).toBe("某人");
  });
});
