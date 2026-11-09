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
  // 票 327：被关注状态不再写成"对方关注了你"——那是叙述，用户能做的动作叫「回关」。
  if (relation === 3) return "回关";
  if (relation === 2) return "已关注";
  return "未关注";
}

export function getRelationActionLabel(relation?: number | null) {
  if (isFollowing(relation)) return "取消关注";
  if (relation === 3) return "回关";
  return "关注";
}

export function getAuthorButtonLabel(relation?: number | null) {
  if (relation === 4) return "互相关注";
  // 同上：主页那颗按钮在 relation=3 时是「回关」，不是一个状态描述。
  if (relation === 3) return "回关";
  if (relation === 2) return "已关注";
  return "关注作者";
}
