import type { UserProfileUpdatePayload } from "~/types/api";

/**
 * 账号设置页两条提交路径的载荷构造（评审票 248 / P1-4）。
 *
 * 病根：`stranger_msg_enabled`（陌生人私信开关）同时被「保存资料」与「保存隐私设置」
 * 两条路径写。资料卡一保存就顺手把隐私开关提交一遍，两个按钮争同一个字段，
 * 谁后点谁生效；而用户看到的是两张卡各自的"已保存"。
 *
 * 收敛后的分工：**隐私卡负责 `stranger_msg_enabled`，资料卡不再附带**。
 * 纯函数放这里是为了能单测锁住「保存资料不发 stranger 字段」这条约定。
 */

/** 资料卡里参与提交的字段（头像单独走 options）。 */
export interface ProfileFormForUpdate {
  username: string;
  nickname: string;
  abstract: string;
  favorites_visibility: boolean;
  followers_visibility: boolean;
  fans_visibility: boolean;
  home_style_id?: string | null;
}

export interface ProfileUpdateExtras {
  /** 已归一化的偏好标签 ID。 */
  likeTagIds: string[];
  /**
   * 头像处理：`undefined` = 不改（不传字段）；`null` = 移除头像；
   * 字符串 = 新上传图片的标识。
   */
  avatarImageId?: string | null;
}

/**
 * 构造 `PUT /api/users/info` 的**资料**载荷。
 *
 * 注意：这里刻意**不**带 `stranger_msg_enabled` —— 它只属于隐私卡
 * （{@link buildPrivacyUpdatePayload}）。
 */
export function buildProfileUpdatePayload(
  form: ProfileFormForUpdate,
  extras: ProfileUpdateExtras,
): UserProfileUpdatePayload {
  const payload: UserProfileUpdatePayload = {
    username: form.username.trim() || null,
    nickname: form.nickname.trim() || null,
    abstract: form.abstract.trim() || null,
    like_tag_ids: extras.likeTagIds,
    favorites_visibility: form.favorites_visibility,
    followers_visibility: form.followers_visibility,
    fans_visibility: form.fans_visibility,
    home_style_id: form.home_style_id || null,
  };

  if (extras.avatarImageId !== undefined) {
    payload.avatar_image_id = extras.avatarImageId;
  }

  return payload;
}

/**
 * 构造 `PUT /api/users/info` 的**隐私**载荷。
 *
 * 只提交陌生人私信开关；这是该字段唯一的提交路径（默认值语义由后端负责：
 * 不传不改，`false` 是合法值）。
 */
export function buildPrivacyUpdatePayload(strangerMsgEnabled: boolean): UserProfileUpdatePayload {
  return { stranger_msg_enabled: strangerMsgEnabled };
}
