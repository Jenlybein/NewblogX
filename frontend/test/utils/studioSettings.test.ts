import { describe, expect, it } from "vitest";

import {
  buildPrivacyUpdatePayload,
  buildProfileUpdatePayload,
  type ProfileFormForUpdate,
} from "~/utils/studioSettings";

/**
 * 账号设置页的载荷分工（评审 #248 / P1-4）。
 *
 * 病根：`stranger_msg_enabled` 同时被「保存资料」与「保存隐私设置」两条路径写，
 * 谁后点谁生效，而用户看到的是两张卡各自的"已保存"。
 * 约定：**隐私卡负责它，资料卡不再附带** —— 这条用例就是锁约定本身。
 */

const form: ProfileFormForUpdate = {
  username: "  reader  ",
  nickname: " 读者 ",
  abstract: " 简介 ",
  favorites_visibility: false,
  followers_visibility: true,
  fans_visibility: false,
};

describe("buildProfileUpdatePayload：保存资料不发 stranger 字段", () => {
  it("载荷里没有 stranger_msg_enabled（该字段只归隐私卡）", () => {
    const payload = buildProfileUpdatePayload(form, { likeTagIds: ["10", "11"] });

    expect("stranger_msg_enabled" in payload).toBe(false);
    expect(Object.keys(payload)).not.toContain("stranger_msg_enabled");
  });

  it("资料字段照常提交：去空白、空串变 null、标签与可见性原样带过", () => {
    const payload = buildProfileUpdatePayload(form, { likeTagIds: ["10", "11"] });

    expect(payload.username).toBe("reader");
    expect(payload.nickname).toBe("读者");
    expect(payload.abstract).toBe("简介");
    expect(payload.like_tag_ids).toEqual(["10", "11"]);
    expect(payload.favorites_visibility).toBe(false);
    expect(payload.followers_visibility).toBe(true);
    expect(payload.fans_visibility).toBe(false);
  });

  it("空白资料回落成 null（后端把 null 当「不设置」）", () => {
    const payload = buildProfileUpdatePayload(
      { ...form, username: "", nickname: "   ", abstract: "" },
      { likeTagIds: [] },
    );

    expect(payload.username).toBeNull();
    expect(payload.nickname).toBeNull();
    expect(payload.abstract).toBeNull();
  });

  it("头像三态：undefined = 不改（不传字段）；null = 移除；字符串 = 新图", () => {
    const unchanged = buildProfileUpdatePayload(form, { likeTagIds: [] });
    expect("avatar_image_id" in unchanged).toBe(false);

    const removed = buildProfileUpdatePayload(form, { likeTagIds: [], avatarImageId: null });
    expect(removed.avatar_image_id).toBeNull();

    const replaced = buildProfileUpdatePayload(form, { likeTagIds: [], avatarImageId: "9001" });
    expect(replaced.avatar_image_id).toBe("9001");
  });
});

describe("buildPrivacyUpdatePayload：隐私卡只提交陌生人私信开关", () => {
  it("载荷只有 stranger_msg_enabled 一个字段（不夹带资料字段）", () => {
    expect(buildPrivacyUpdatePayload(true)).toEqual({ stranger_msg_enabled: true });
    expect(buildPrivacyUpdatePayload(false)).toEqual({ stranger_msg_enabled: false });
    expect(Object.keys(buildPrivacyUpdatePayload(false))).toEqual(["stranger_msg_enabled"]);
  });
});
