export type UserRelation = 0 | 1 | 2 | 3 | 4;

export function isFollowing(relation?: number | null) {
  return relation === 2 || relation === 4;
}

export function isFollowedBy(relation?: number | null) {
  return relation === 3 || relation === 4;
}

export function isMutualFollow(relation?: number | null) {
  return relation === 4;
}

export function getRelationLabel(relation?: number | null) {
  if (relation === 4) return "互相关注";
  if (relation === 3) return "对方关注了你";
  if (relation === 2) return "已关注";
  return "未关注";
}

export function getRelationActionLabel(relation?: number | null) {
  if (isFollowing(relation)) return "取消关注";
  if (relation === 3) return "回关";
  return "关注";
}

/**
 * 粉丝语境的动作文案。
 *
 * 粉丝列表里的人是"关注了你"的一方，所以撤掉关注时要区分两个方向：
 * 互相关注（4）撤掉的是**你回关的那一半** →「取消回关」；单向已关注（2）→「取消关注」；
 * 其余（对方还没被你关注，含 3 / 0 / 1）→「回关」。
 *
 * 与 {@link getRelationActionLabel} 并存而不是替换：公开主页 `/users/[id]` 的按钮
 * 文案是"你与这个人的关系"，两种语境不能被同一个函数糊在一起。
 */
export function getFansRelationActionLabel(relation?: number | null) {
  if (relation === 4) return "取消回关";
  if (relation === 2) return "取消关注";
  return "回关";
}

export function getAuthorButtonLabel(relation?: number | null) {
  if (relation === 4) return "互相关注";
  if (relation === 3) return "对方关注了你";
  if (relation === 2) return "已关注";
  return "关注作者";
}
