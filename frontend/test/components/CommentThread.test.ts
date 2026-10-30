import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import CommentThread from "~/components/comment/CommentThread.vue";
import type { CommentRootItem } from "~/types/api";

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

describe("CommentThread（评论楼层级）", () => {
  it("渲染楼层：作者、内容、状态标签与操作按钮", () => {
    const wrapper = mount(CommentThread, { props: { comments: [makeComment()] } });

    expect(wrapper.findAll(".comment")).toHaveLength(1);
    expect(wrapper.get(".comment__author").text()).toBe("读者甲");
    expect(wrapper.get(".comment__content").text()).toContain("这是一条评论");
    // 待审核状态用语义写法的独立标签（不再借用 Naive 的 warning 橙色）。
    expect(wrapper.get(".comment__status").text()).toBe("审核中");
    expect(wrapper.findAll(".comment-action")).toHaveLength(3);
  });

  it("展开回复时发出 loadReplies，并切换按钮文案", async () => {
    const wrapper = mount(CommentThread, { props: { comments: [makeComment()] } });
    const expandButton = wrapper.findAll(".comment-action")[2]!;

    expect(expandButton.text()).toContain("展开 2 条回复");
    await expandButton.trigger("click");

    expect(wrapper.emitted("loadReplies")?.[0]).toEqual(["c1"]);
    expect(wrapper.findAll(".comment-action")[2]!.text()).toContain("收起回复");
  });

  it("没有回复时不出展开按钮；展开后回复列表按嵌套层级渲染", async () => {
    const withReplies = mount(CommentThread, {
      props: {
        comments: [
          {
            ...makeComment({ reply_count: 1 }),
            replies: [
              {
                id: "r1",
                user_id: "u2",
                user_nickname: "读者乙",
                user_avatar: "",
                content: "楼中楼回复",
                created_at: "2026-10-30T11:00:00.000Z",
                digg_count: 0,
                reply_user_nickname: "读者甲",
              },
            ],
          },
        ],
      },
    });

    // 回复默认收起；点"展开"后才渲染楼中楼（并按 .comment--reply 嵌套缩进）。
    const expandButton = withReplies.findAll(".comment-action")[2]!;
    await expandButton.trigger("click");

    expect(withReplies.findAll(".comment")).toHaveLength(2);
    expect(withReplies.get(".comment--reply").text()).toContain("楼中楼回复");

    const noReplies = mount(CommentThread, { props: { comments: [makeComment({ reply_count: 0 })] } });
    expect(noReplies.findAll(".comment-action")[2]).toBeUndefined();
  });
});
