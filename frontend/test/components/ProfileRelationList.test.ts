import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ProfileRelationList from "~/components/profile/ProfileRelationList.vue";
import type { FanUserItem, FollowUserItem } from "~/types/api";

/**
 * 关系列表（公开主页 / 个人中心的关注、粉丝三处共用）。
 *
 * 这一票（#327）把个人中心的粉丝、关注页从 naive-ui 的 NList 换成它，于是它多了一个
 * 语境参数：
 *   - `mode: "relation"`（默认，公开主页）：按钮说"你与这个人的关系"；
 *   - `mode: "fans"`（粉丝列表）：对方本来就关注着你，撤掉的是"你回关的那一半"，
 *     互相关注时文案必须是「取消回关」。
 *
 * 用例只锁用户看得见的文案与抛给页面的事件；样式与布局不在这一层断言。
 */

function fansItem(overrides: Partial<FanUserItem> = {}): FanUserItem {
  return {
    fans_user_id: "u1",
    fans_nickname: "小粉丝",
    fans_avatar: "",
    fans_abstract: "一个关注了你很久的人",
    follow_time: "2026-11-01T10:00:00Z",
    relation: 4,
    ...overrides,
  };
}

function followItem(overrides: Partial<FollowUserItem> = {}): FollowUserItem {
  return {
    followed_user_id: "u2",
    followed_nickname: "被关注的人",
    followed_avatar: "",
    followed_abstract: "",
    follow_time: "2026-11-02T10:00:00Z",
    relation: 2,
    ...overrides,
  };
}

function mountList(items: Array<FollowUserItem | FanUserItem>, mode?: "relation" | "fans") {
  return mount(ProfileRelationList, {
    props: { title: "粉丝列表", items, ...(mode ? { mode } : {}) },
    global: {
      stubs: {
        NuxtLink: {
          props: { to: { type: [String, Object], default: "" } },
          template: "<a><slot /></a>",
        },
      },
    },
  });
}

/** 每张卡片上的动作按钮文案（顺序与卡片一致）。 */
function actionLabels(wrapper: ReturnType<typeof mountList>): string[] {
  return wrapper
    .findAll(".profile-relation-card__actions button")
    .map((button) => button.text());
}

describe("关系列表：动作文案按语境切换", () => {
  it("默认（公开主页）沿用关系文案：互相关注 → 取消关注", () => {
    const wrapper = mountList([followItem({ relation: 4 }), followItem({ relation: 3 }), followItem({ relation: 2 })]);

    expect(actionLabels(wrapper)).toEqual(["取消关注", "回关", "取消关注"]);
  });

  it("粉丝语境：互相关注 → 取消回关，单向已关注 → 取消关注，其余 → 回关", () => {
    const wrapper = mountList(
      [fansItem({ relation: 4 }), fansItem({ relation: 2 }), fansItem({ relation: 3 }), fansItem({ relation: 0 })],
      "fans",
    );

    expect(actionLabels(wrapper)).toEqual(["取消回关", "取消关注", "回关", "回关"]);
  });

  it("关系 chip 不随语境变：两种 mode 都用 getRelationLabel", () => {
    const relationMode = mountList([fansItem({ relation: 4 })]);
    const fansMode = mountList([fansItem({ relation: 4 })], "fans");

    expect(relationMode.get(".blueprint-chip").text()).toBe("互相关注");
    expect(fansMode.get(".blueprint-chip").text()).toBe("互相关注");
  });
});

describe("关系列表：动作事件交给页面处理", () => {
  it("点动作按钮抛出 toggleFollow(id, relation)，粉丝条目用 fans_user_id", async () => {
    const wrapper = mountList([fansItem({ fans_user_id: "u7", relation: 4 })], "fans");

    await wrapper.get(".profile-relation-card__actions button").trigger("click");

    expect(wrapper.emitted("toggleFollow")).toEqual([["u7", 4]]);
  });

  it("点动作按钮抛出 toggleFollow(id, relation)，关注条目用 followed_user_id", async () => {
    const wrapper = mountList([followItem({ followed_user_id: "u8", relation: 2 })]);

    await wrapper.get(".profile-relation-card__actions button").trigger("click");

    expect(wrapper.emitted("toggleFollow")).toEqual([["u8", 2]]);
  });
});
