import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ProfileRelationList from "~/components/profile/ProfileRelationList.vue";
import type { RelationItem } from "~/types/api";

/**
 * 关系列表（公开主页 `/users/[id]` 的关注 / 粉丝分栏）。
 *
 * 票 327 把三个视图的条目合并成同一个形状 `RelationItem`（`user_id / nickname /
 * avatar / abstract / follow_time / relation`），所以这个组件不再按字段名分支，
 * 也不再需要"语境"参数：动作文案统一表达"你与这个人的关系"。
 *
 * 用例只锁用户看得见的文案与抛给页面的事件；样式与布局不在这一层断言。
 */

function relationItem(overrides: Partial<RelationItem> = {}): RelationItem {
  return {
    user_id: "u1",
    nickname: "小粉丝",
    avatar: "",
    abstract: "一个关注了你很久的人",
    follow_time: "2026-11-01T10:00:00Z",
    relation: 4,
    ...overrides,
  };
}

function mountList(items: RelationItem[]) {
  return mount(ProfileRelationList, {
    props: { title: "粉丝列表", items },
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

/**
 * 公开主页只承担展示：没有取消关注 / 回关按钮。
 * 关系动作属于个人中心，出现在别人的主页上等于"在别人的页面改自己的关系"。
 */
describe("关系列表：只展示，不带关系动作", () => {
  it("三种关系都不渲染动作按钮", () => {
    const wrapper = mountList([
      relationItem({ user_id: "a", relation: 4 }),
      relationItem({ user_id: "b", relation: 3 }),
      relationItem({ user_id: "c", relation: 2 }),
    ]);

    expect(wrapper.findAll("button")).toHaveLength(0);
  });

  it("关系 chip 描述客观关系；relation=3 是「回关」而不是「对方关注了你」", () => {
    const wrapper = mountList([relationItem({ user_id: "a", relation: 4 }), relationItem({ user_id: "b", relation: 3 })]);

    expect(wrapper.findAll(".blueprint-chip").map((chip) => chip.text())).toEqual(["互相关注", "回关"]);
  });
});

describe("关系列表：动作事件交给页面处理", () => {
  it("公开主页的关系列表不提供移除粉丝（管理只在个人中心）", async () => {
    const wrapper = mountList([relationItem({ relation: 3 })]);

    expect(wrapper.text()).not.toContain("移除粉丝");
  });
});
