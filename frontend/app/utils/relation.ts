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

export function getAuthorButtonLabel(relation?: number | null) {
  if (relation === 4) return "互相关注";
  if (relation === 3) return "对方关注了你";
  if (relation === 2) return "已关注";
  return "关注作者";
}
