import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import CommentThread from "~/components/comment/CommentThread.vue";
import type { CommentReplyItem, CommentRootItem } from "~/types/api";

/**
 * 评论楼层级（#238 范围 1/2/4/5）。
 *
 * 这里只测组件的对外协议（渲染了什么、点了发什么事件、无障碍名是什么），
 * 不碰页面怎么处理事件（那是 `useArticleComments.test.ts` 的事）：
 *
 *  - 子评论点赞必须**可点**（此前是纯文本 `<span>点赞 N</span>`，后端其实早就支持二级点赞）；
 *  - 子评论「回复」要发**被回复的那条评论 id**（此前发根评论 id，回复落错楼层）；
 *  - 点赞/回复是图标版：实心/空心两态 + 数字、回复箭头，全部带 `aria-label` 且是原生按钮；
 *  - 「展开 N 条回复」是胶囊按钮（浅底 + 主题色文字 + chevron）。
 */

function makeComment(overrides: Partial<CommentRootItem> = {}): CommentRootItem {
  return {
    id: "c1",
    user_id: "u1",
    user_nickname: "读者甲",
    user_avatar: "",
    content: "这是一条评论",
    created_at: "2026-10-30T10:00:00.000Z",
    digg_count: 3,
    is_digg: false,
    reply_count: 2,
    status: 1,
    ...overrides,
  } as CommentRootItem;
}

function makeReply(overrides: Partial<CommentReplyItem> = {}): CommentReplyItem {
  return {
    id: "r1",
    user_id: "u2",
    user_nickname: "读者乙",
    user_avatar: "",
    content: "楼中楼回复",
    created_at: "2026-10-30T11:00:00.000Z",
    digg_count: 1,
    is_digg: false,
    status: 1,
    reply_user_nickname: "读者甲",
    ...overrides,
  } as CommentReplyItem;
}

function mountWithReplies(comment: Partial<CommentRootItem> = {}, replies: CommentReplyItem[] = [makeReply()]) {
  return mount(CommentThread, {
    props: {
      comments: [{ ...makeComment(comment), replies }],
    },
  });
}

/** 展开楼层的回复，返回渲染后的 wrapper（回复行默认收起）。 */
async function mountExpanded(comment: Partial<CommentRootItem> = {}, replies: CommentReplyItem[] = [makeReply()]) {
  const wrapper = mountWithReplies(comment, replies);
  await wrapper.get(".comment-capsule").trigger("click");
  return wrapper;
}

describe("CommentThread（评论楼层级）", () => {
  it("渲染楼层：作者、内容、状态标签与操作按钮", () => {
    const wrapper = mount(CommentThread, { props: { comments: [makeComment()] } });

    expect(wrapper.findAll(".comment")).toHaveLength(1);
    expect(wrapper.get(".comment__author").text()).toBe("读者甲");
    expect(wrapper.get(".comment__content").text()).toContain("这是一条评论");
    // 待审核状态用语义写法的独立标签（不再借用 Naive 的 warning 橙色）。
    expect(wrapper.get(".comment__status").text()).toBe("审核中");
    // 根评论的操作区：点赞 + 回复 + 展开胶囊（不再有纯文本操作）。
    expect(wrapper.findAll(".comment-action")).toHaveLength(2);
    expect(wrapper.find(".comment-capsule").exists()).toBe(true);
  });

  it("根评论点赞是图标按钮：空心/实心两态 + 数字，aria-label 两态清晰", async () => {
    const wrapper = mount(CommentThread, { props: { comments: [makeComment({ is_digg: false, digg_count: 7 })] } });
    const diggButton = wrapper.get(".comment-action--digg");

    expect(diggButton.element.tagName).toBe("BUTTON");
    expect(diggButton.attributes("type")).toBe("button");
    expect(diggButton.attributes("aria-label")).toBe("点赞");
    expect(diggButton.attributes("aria-pressed")).toBe("false");
    expect(diggButton.get(".comment-action__count").text()).toBe("7");
    // 未点赞是空心图标（filled 变体带 -filled 的名字，通过 svg 的 aria-hidden 判断图标存在即可）。
    expect(diggButton.findAll("svg")).toHaveLength(1);

    await diggButton.trigger("click");
    expect(wrapper.emitted("digg")?.[0]).toEqual(["c1", false]);

    const digged = mount(CommentThread, { props: { comments: [makeComment({ is_digg: true })] } });
    expect(digged.get(".comment-action--digg").attributes("aria-label")).toBe("取消点赞");
    expect(digged.get(".comment-action--digg").attributes("aria-pressed")).toBe("true");
  });

  it("回复按钮是箭头图标 + 原生按钮，点了发根评论 id", async () => {
    const wrapper = mount(CommentThread, { props: { comments: [makeComment()] } });
    const replyButton = wrapper.get(".comment-action--reply");

    expect(replyButton.element.tagName).toBe("BUTTON");
    expect(replyButton.attributes("aria-label")).toBe("回复");
    expect(replyButton.find("svg").exists()).toBe(true);
    expect(replyButton.text()).toContain("回复");

    await replyButton.trigger("click");
    expect(wrapper.emitted("reply")?.[0]).toEqual(["c1"]);
  });

  it("「展开 N 条回复」是胶囊按钮：浅底 + 主题色文字 + chevron，展开后文案与 aria 状态一起变", async () => {
    const wrapper = mount(CommentThread, { props: { comments: [makeComment({ reply_count: 2 })] } });
    const capsule = wrapper.get(".comment-capsule");

    expect(capsule.element.tagName).toBe("BUTTON");
    expect(capsule.attributes("aria-expanded")).toBe("false");
    expect(capsule.text()).toContain("展开 2 条回复");
    expect(capsule.find(".comment-capsule__chevron").exists()).toBe(true);

    await capsule.trigger("click");

    expect(wrapper.emitted("loadReplies")?.[0]).toEqual(["c1"]);
    expect(wrapper.get(".comment-capsule").text()).toContain("收起回复");
    expect(wrapper.get(".comment-capsule").attributes("aria-expanded")).toBe("true");
  });

  it("没有回复时不出现展开胶囊", () => {
    const wrapper = mount(CommentThread, { props: { comments: [makeComment({ reply_count: 0 })] } });

    expect(wrapper.find(".comment-capsule").exists()).toBe(false);
  });

  it("展开后回复列表按嵌套层级渲染", async () => {
    const wrapper = await mountExpanded({ reply_count: 1 });

    expect(wrapper.findAll(".comment")).toHaveLength(2);
    expect(wrapper.get(".comment--reply").text()).toContain("楼中楼回复");
  });
});

describe("CommentThread：子评论互动（不再落到根楼层）", () => {
  it("子评论点赞可点：按**子评论 id** 发 digg，并带上它自己的点赞态", async () => {
    const wrapper = await mountExpanded();
    const replyRow = wrapper.get(".comment--reply");
    const diggButton = replyRow.get(".comment-action--digg");

    expect(diggButton.element.tagName).toBe("BUTTON");
    expect(diggButton.attributes("aria-label")).toBe("点赞");
    expect(diggButton.get(".comment-action__count").text()).toBe("1");

    await diggButton.trigger("click");

    expect(wrapper.emitted("digg")?.[0]).toEqual(["r1", false]);
  });

  it("子评论已点赞：实心图标与「取消点赞」语义，点后发取消", async () => {
    const wrapper = await mountExpanded({}, [makeReply({ is_digg: true, digg_count: 4 })]);
    const diggButton = wrapper.get(".comment--reply").get(".comment-action--digg");

    expect(diggButton.attributes("aria-label")).toBe("取消点赞");
    expect(diggButton.attributes("aria-pressed")).toBe("true");

    await diggButton.trigger("click");

    expect(wrapper.emitted("digg")?.[0]).toEqual(["r1", true]);
  });

  it("子评论「回复」发的是子评论 id（不是根评论 c1）", async () => {
    const wrapper = await mountExpanded();

    await wrapper.get(".comment--reply").get(".comment-action--reply").trigger("click");

    expect(wrapper.emitted("reply")?.[0]).toEqual(["r1"]);
  });
});
