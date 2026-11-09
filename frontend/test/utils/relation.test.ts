import { describe, expect, it } from "vitest";
import {
  getAuthorButtonLabel,
  getRelationActionLabel,
  getRelationLabel,
  isFollowedBy,
  isFollowing,
  isMutualFollow,
} from "~/utils/relation";

describe("relation utils", () => {
  it("identifies one-way and mutual follow states", () => {
    expect(isFollowing(2)).toBe(true);
    expect(isFollowing(4)).toBe(true);
    expect(isFollowing(3)).toBe(false);
    expect(isFollowedBy(3)).toBe(true);
    expect(isFollowedBy(4)).toBe(true);
    expect(isMutualFollow(4)).toBe(true);
    expect(isMutualFollow(null)).toBe(false);
  });

  it("returns user-facing labels for relation states", () => {
    expect(getRelationLabel(4)).toBe("互相关注");
    // 票 327：被关注状态不再写成"对方关注了你"——那是叙述，用户能做的动作叫「回关」。
    expect(getRelationLabel(3)).toBe("回关");
    expect(getRelationLabel(2)).toBe("已关注");
    expect(getRelationLabel(0)).toBe("未关注");
  });

  it("returns action labels that match the next available action", () => {
    expect(getRelationActionLabel(4)).toBe("取消关注");
    expect(getRelationActionLabel(2)).toBe("取消关注");
    expect(getRelationActionLabel(3)).toBe("回关");
    expect(getRelationActionLabel(undefined)).toBe("关注");
    expect(getAuthorButtonLabel(undefined)).toBe("关注作者");
  });

  it("主页那颗按钮在 relation=3 时说的是「回关」而不是状态叙述", () => {
    expect(getAuthorButtonLabel(3)).toBe("回关");
    expect(getAuthorButtonLabel(2)).toBe("已关注");
    expect(getAuthorButtonLabel(4)).toBe("互相关注");
    // 同一句话只在一处声明：状态标签与按钮标签必须一致，否则主页会出现
    // 「对方关注了你」的按钮（点了其实是在回关）。
    expect(getAuthorButtonLabel(3)).toBe(getRelationLabel(3));
  });
});
