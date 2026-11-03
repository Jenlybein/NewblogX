import { describe, expect, it } from "vitest";

import { describeArticleAction } from "~/utils/articleActions";

/**
 * 阅读页互动条的文案（#236 / 决定清单第 2 节）。
 *
 * 互动条的四个动作是**纯图标 + 数字**（右栏 320px 宽、窄屏底部条还要塞安全区），
 * 可见文字省掉了，所以读屏名称与悬停提示就是唯一的语义出口 —— 它们是可测的规格：
 *   1. 每个动作都要说出"是什么"（点赞 / 收藏 / 分享 / 评论）；
 *   2. 已赞 / 已藏时说出"点它会撤销"（否则屏幕阅读器只能读到"点赞"）；
 *   3. 计数跟当前数字走，而不是写死的文案；
 *   4. 点赞请求在途时说明"处理中"，不让读屏读到会误导的旧态。
 *
 * 这里断言的是**字面文案**（规格），不是把实现里的拼接口径再算一遍。
 */

describe("describeArticleAction：四个动作的读屏名称与悬停提示", () => {
  it("点赞：未赞说「点赞」，已赞说「取消点赞」，计数来自入参", () => {
    expect(describeArticleAction({ kind: "digg", count: 12, active: false })).toEqual({
      label: "点赞，当前 12 个赞",
      title: "点赞",
    });
    expect(describeArticleAction({ kind: "digg", count: 12, active: true })).toEqual({
      label: "取消点赞，当前 12 个赞",
      title: "取消点赞",
    });
  });

  it("收藏：未藏说「收藏」，已藏说「取消收藏」", () => {
    expect(describeArticleAction({ kind: "favor", count: 5, active: false })).toEqual({
      label: "收藏，当前 5 次收藏",
      title: "收藏",
    });
    expect(describeArticleAction({ kind: "favor", count: 5, active: true })).toEqual({
      label: "已收藏，点按可移除，当前 5 次收藏",
      title: "已收藏",
    });
  });

  it("分享：没有计数概念，也不暗示会调接口", () => {
    expect(describeArticleAction({ kind: "share", count: 0 })).toEqual({
      label: "分享这篇文章",
      title: "分享",
    });
  });

  it("评论：说明点它会把焦点送进评论框", () => {
    expect(describeArticleAction({ kind: "comment", count: 7 })).toEqual({
      label: "跳到评论区写评论，当前 7 条评论",
      title: "评论",
    });
  });
});

describe("describeArticleAction：边界与在途态", () => {
  it("计数为负 / 非数字时按 0 说，不把 NaN 念给读屏", () => {
    expect(describeArticleAction({ kind: "digg", count: -3, active: false }).label).toBe("点赞，当前 0 个赞");
    expect(describeArticleAction({ kind: "comment", count: Number.NaN }).label).toBe(
      "跳到评论区写评论，当前 0 条评论",
    );
  });

  it("点赞在途时说「处理中」：此时按下去会切到哪一侧还不确定", () => {
    expect(describeArticleAction({ kind: "digg", count: 9, active: true, pending: true })).toEqual({
      label: "点赞处理中，当前 9 个赞",
      title: "点赞处理中",
    });
  });

  it("千级以上计数沿用站内 formatCount 口径（1.2k），不另造一套缩写", () => {
    expect(describeArticleAction({ kind: "digg", count: 1234, active: false }).label).toBe("点赞，当前 1.2k 个赞");
    expect(describeArticleAction({ kind: "comment", count: 12000, active: false }).label).toBe(
      "跳到评论区写评论，当前 1.2w 条评论",
    );
  });
});
