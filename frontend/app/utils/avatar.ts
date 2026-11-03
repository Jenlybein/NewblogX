import { shapeImage, type ShapeImageDeps, type ShapeImageOptions } from "./imageShaping";

/**
 * 头像定形参数：居中裁成 1:1、降采样到 256×256、输出 PNG。
 * 这是「头像定形」的唯一参数真相（见 GLOSSARY）；上传原图的做法已退场。
 */
export const AVATAR_SHAPE_OPTIONS: ShapeImageOptions = {
  target: { width: 256, height: 256 },
  outputType: "image/png",
};

/** 头像定形入口：返回可直接上传的新 PNG，原图不上传。 */
export function shapeAvatar(file: Blob, deps?: ShapeImageDeps): Promise<Blob> {
  return shapeImage(file, AVATAR_SHAPE_OPTIONS, deps);
}

const AVATAR_FIELD_CANDIDATES = [
  "avatar",
  "user_avatar",
  "avatar_url",
  "author_avatar",
  "action_user_avatar",
  "receiver_avatar",
  "followed_avatar",
  "fans_avatar",
] as const;

const NAME_FIELD_CANDIDATES = [
  "nickname",
  "user_nickname",
  "username",
  "user_name",
  "author_name",
  "action_user_nickname",
  "receiver_nickname",
  "followed_nickname",
  "fans_nickname",
  "name",
] as const;

function resolveAssetProxyBase() {
  try {
    const config = useRuntimeConfig();
    const value = String(config.public.assetProxyBase || "").trim();
    return value || "/uploads";
  } catch {
    return "/uploads";
  }
}

function normalizeAvatarUrl(raw: string): string {
  const value = raw.trim();
  if (!value) {
    return "";
  }

  if (/^(?:https?:)?\/\//i.test(value) || value.startsWith("data:") || value.startsWith("blob:")) {
    return value;
  }

  const assetProxyBase = resolveAssetProxyBase();

  // 后端历史数据里头像常是相对路径，统一补上资源基址避免 404/CORS。
  //
  // 这里的取值顺序有意义：先判"已经带了基址"，否则 "/uploads/a.png" 这种
  // 会被再拼一次变成 "/uploads/uploads/a.png"。
  // 曾经还有一条把 "/_backend/x" 改写成 "{基址}/x" 的兼容分支，
  // 随着 nginx 的 /_backend 别名一起删掉了（S5.5）。
  if (value.startsWith(`${assetProxyBase}/`)) {
    return value;
  }
  if (value.startsWith("/")) {
    return `${assetProxyBase}${value}`;
  }

  return `${assetProxyBase}/${value.replace(/^\/+/, "")}`;
}

export function resolveAvatarUrl(input: unknown): string {
  if (typeof input === "string") {
    return normalizeAvatarUrl(input);
  }

  if (!input || typeof input !== "object") {
    return "";
  }

  const candidate = input as Record<string, unknown>;
  for (const key of AVATAR_FIELD_CANDIDATES) {
    const value = candidate[key];
    if (typeof value !== "string") {
      continue;
    }
    const normalized = value.trim();
    if (normalized) {
      return normalizeAvatarUrl(normalized);
    }
  }

  return "";
}

export function resolveDisplayName(input: unknown): string {
  if (typeof input === "string") {
    return input.trim();
  }

  if (!input || typeof input !== "object") {
    return "";
  }

  const candidate = input as Record<string, unknown>;
  for (const key of NAME_FIELD_CANDIDATES) {
    const value = candidate[key];
    if (typeof value !== "string") {
      continue;
    }
    const normalized = value.trim();
    if (normalized) {
      return normalized;
    }
  }

  return "";
}

export function resolveAvatarInitial(nameInput: unknown, fallback = "?"): string {
  const name = resolveDisplayName(nameInput);
  const first = [...name][0];
  if (!first) {
    return fallback;
  }
  return /^[a-zA-Z]$/u.test(first) ? first.toUpperCase() : first;
}
