/**
 * 后台头像的兜底口径（决定清单 §6）。
 *
 * 读者站那份 `frontend/app/utils/avatar.ts` 负责"定形 + 资源基址 + 从对象里猜字段"，
 * 后台这里只需要两件事：**把地址补全**、**无头像时该显示什么**。
 *
 * 为什么不能照抄读者站的 `(nickname || username).slice(0, 1)`：后台列表里
 * 一半账号的用户名是邮箱（`1813288952@qq.com`），首位字符会画出一个无意义的 `1`。
 * 所以邮箱形式的昵称/用户名一律不取首字符，交给调用方渲染人形图标。
 */

/** 与 nginx 的资源代理基址一致（读者站同口径，后台没有单独的 runtimeConfig 键）。 */
const ASSET_PROXY_BASE = "/uploads";

/** 邮箱形式的用户名：不取首字符（会出现 "1" 这种无意义头像）。 */
const EMAIL_LIKE = /^[^@\s]+@[^@\s]+$/u;

/** 是否看起来像一个邮箱地址（用于决定"能不能拿首字符当头像"）。 */
export function isEmailLikeName(input: unknown): boolean {
  return typeof input === "string" && EMAIL_LIKE.test(input.trim());
}

/** 把后台拿到的头像地址补成全地址；空值返回空串。 */
export function resolveAdminAvatarUrl(input: unknown): string {
  if (typeof input !== "string") {
    return "";
  }

  const value = input.trim();
  if (!value) {
    return "";
  }

  if (/^(?:https?:)?\/\//iu.test(value) || value.startsWith("data:") || value.startsWith("blob:")) {
    return value;
  }

  // 先判"已经带了基址"，否则 "/uploads/a.png" 会被再拼一次。
  if (value.startsWith(`${ASSET_PROXY_BASE}/`)) {
    return value;
  }
  if (value.startsWith("/")) {
    return `${ASSET_PROXY_BASE}${value}`;
  }

  return `${ASSET_PROXY_BASE}/${value.replace(/^\/+/u, "")}`;
}

/** 无头像时要画的首字符；邮箱形式与空名一律返回空串（调用方改画人形图标）。 */
export function resolveAdminAvatarInitial(input: unknown, fallback = ""): string {
  const name = typeof input === "string" ? input.trim() : "";
  if (!name || isEmailLikeName(name)) {
    return fallback;
  }

  const first = [...name][0];
  if (!first) {
    return fallback;
  }

  return /[a-z]/iu.test(first) ? first.toUpperCase() : first;
}
