import { getRequestURL, getRouterParam, proxyRequest } from "h3";
import type { H3Event } from "h3";

function normalizeOrigin(origin: string) {
  return origin.endsWith("/") ? origin : `${origin}/`;
}

function normalizePath(path: string) {
  return path.replace(/^\/+/, "");
}

function splitPathSegments(path: string) {
  return normalizePath(path).split("/").filter(Boolean);
}

export function resolveProxyTarget(origin: string, path: string, search: string) {
  const targetUrl = new URL(normalizeOrigin(origin));
  const baseSegments = splitPathSegments(targetUrl.pathname);
  const pathSegments = splitPathSegments(path);

  if (
    baseSegments.length > 0 &&
    pathSegments.length > 0 &&
    baseSegments[baseSegments.length - 1] === pathSegments[0]
  ) {
    pathSegments.shift();
  }

  const targetPath = [...baseSegments, ...pathSegments].join("/");
  targetUrl.pathname = targetPath ? `/${targetPath}` : "/";
  targetUrl.search = search;
  return targetUrl.toString();
}

export function proxyRoutePath(event: H3Event, origin: string, pathPrefix = "") {
  const requestPath = getRouterParam(event, "path") || "";
  const requestUrl = getRequestURL(event);
  const normalizedPrefix = normalizePath(pathPrefix);
  const normalizedRequestPath = normalizePath(requestPath);
  const joinedPath =
    normalizedPrefix && normalizedRequestPath !== normalizedPrefix && !normalizedRequestPath.startsWith(`${normalizedPrefix}/`)
      ? `${normalizedPrefix}/${normalizedRequestPath}`
      : normalizedRequestPath;
  return proxyRequest(event, resolveProxyTarget(origin, joinedPath, requestUrl.search));
}
