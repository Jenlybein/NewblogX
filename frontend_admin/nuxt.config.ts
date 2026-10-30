import { existsSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { defineNuxtConfig } from "nuxt/config";

import { BRAND, brandAbsoluteAssetUrl, brandAdminTitle, brandAssetUrl, brandFullName } from "./app/utils/brand";

const appRoot = dirname(fileURLToPath(import.meta.url));
const hydratedEnvKeys = new Set<string>();


// 只允许注入本应用命名空间下的键（以及 Nuxt 自身的 NUXT_ 键）。
// 历史原因：env 文件曾经从后端配置里抄过来，一旦有人复制了 BLOGX_JWT_SECRET
// 这类后端密钥，它就会被无差别写进前端进程环境。这里用白名单从源头堵住。
const ALLOWED_ENV_PREFIX = ['BLOGX_ADMIN_', 'NUXT_']
function isAllowedEnvKey(key: string) {
  return ALLOWED_ENV_PREFIX.some((prefix) => key.startsWith(prefix))
}

function hydrateProcessEnv(
  relativePath: string,
  options: { overrideHydrated?: boolean } = {},
) {
  const envPath = resolve(appRoot, relativePath);
  if (!existsSync(envPath)) return;

  const source = readFileSync(envPath, "utf8");
  for (const rawLine of source.split(/\r?\n/u)) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) continue;

    const separatorIndex = line.indexOf("=");
    if (separatorIndex <= 0) continue;

    const key = line.slice(0, separatorIndex).trim();
    // 非本应用命名空间的键直接忽略（防止后端密钥泄漏到前端进程环境）
    if (key && !isAllowedEnvKey(key)) {
      continue;
    }
    if (
      !key ||
      (process.env[key] !== undefined &&
        !(options.overrideHydrated && hydratedEnvKeys.has(key)))
    ) {
      continue;
    }

    let value = line.slice(separatorIndex + 1).trim();
    const quoted =
      (value.startsWith('"') && value.endsWith('"')) ||
      (value.startsWith("'") && value.endsWith("'"));
    if (quoted) value = value.slice(1, -1);

    process.env[key] = value;
    hydratedEnvKeys.add(key);
  }
}

function normalizeOrigin(value: string) {
  return value.endsWith("/") ? value.slice(0, -1) : value;
}

function normalizePathSegments(value: string) {
  return value
    .replace(/^\/+|\/+$/gu, "")
    .split("/")
    .filter(Boolean);
}

function joinOriginPath(origin: string, path: string) {
  const targetUrl = new URL(normalizeOrigin(origin));
  const originSegments = normalizePathSegments(targetUrl.pathname);
  const pathSegments = normalizePathSegments(path);

  if (
    originSegments.length > 0 &&
    pathSegments.length > 0 &&
    originSegments.at(-1) === pathSegments[0]
  ) {
    pathSegments.shift();
  }

  targetUrl.pathname = [...originSegments, ...pathSegments].join("/") || "/";
  return normalizeOrigin(targetUrl.toString());
}

function normalizeModuleId(value: string) {
  return value.replace(/\\/gu, "/");
}

function isModulePreloadSourcemapWarning(message: string) {
  return (
    message.includes("[plugin nuxt:module-preload-polyfill]") &&
    message.includes("Sourcemap is likely to be incorrect")
  );
}

const envProfile =
  (
    process.env.NUXT_ENV_PROFILE ||
    process.env.BLOGX_ADMIN_ENV_PROFILE ||
    process.env.BLOGX_WEB_ENV_PROFILE ||
    "local-local"
  ).trim() || "local-local";
hydrateProcessEnv("env/common.env");
hydrateProcessEnv(`env/${envProfile}.env`, { overrideHydrated: true });

const apiBase =
  String(
    process.env.NUXT_PUBLIC_API_BASE ||
      process.env.BLOGX_ADMIN_API_BASE ||
      process.env.BLOGX_WEB_API_BASE ||
      "/api",
  ).trim() || "/api";
const adminBaseURL =
  String(process.env.NUXT_APP_BASE_URL || process.env.BLOGX_ADMIN_BASE_URL || "/admin/").trim() || "/admin/";
const apiUpstream = normalizeOrigin(
  process.env.NUXT_API_UPSTREAM ||
    process.env.BLOGX_ADMIN_API_UPSTREAM ||
    process.env.BLOGX_WEB_API_UPSTREAM ||
    process.env.NUXT_API_ORIGIN ||
    "http://127.0.0.1:8080",
);
const apiProxyTarget = joinOriginPath(apiUpstream, apiBase);
const devProxy = apiBase.startsWith("/")
  ? {
      [apiBase]: {
        target: apiProxyTarget,
        changeOrigin: true,
        ws: true,
      },
    }
  : {};

const adminSiteUrl =
  process.env.NUXT_PUBLIC_SITE_URL ||
  process.env.BLOGX_ADMIN_SITE_URL ||
  "http://localhost:3001/admin";
const adminOgImage = brandAbsoluteAssetUrl(adminSiteUrl, "og");

const naiveUiFoundationPackages = [
  "/node_modules/vueuc/",
  "/node_modules/seemly/",
  "/node_modules/evtd/",
  "/node_modules/vdirs/",
  "/node_modules/@css-render/",
  "/node_modules/css-render/",
];

// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: "2025-07-15",
  telemetry: false,
  devtools: { enabled: true },
  // 后台此前完全没有 app.head（标签页只有 URL、没有 title/lang/icon）。这里补齐：
  // 标题 / 语言 / 描述 / 图标（含 /admin/ baseURL）；OG 绝对地址放在 app.vue 的
  // useHead（需要运行时 siteUrl）。
  app: {
    baseURL: adminBaseURL,
    head: {
      htmlAttrs: { lang: "zh-CN" },
      title: brandAdminTitle(),
      meta: [
        { name: "description", content: BRAND.description },
        { property: "og:site_name", content: brandFullName() },
        { property: "og:type", content: "website" },
        { property: "og:title", content: brandFullName() },
        { property: "og:description", content: BRAND.description },
        { property: "og:image", content: adminOgImage },
        { property: "og:image:width", content: "1200" },
        { property: "og:image:height", content: "630" },
        { property: "og:image:alt", content: brandFullName() },
        { name: "twitter:card", content: "summary_large_image" },
        { name: "twitter:title", content: brandFullName() },
        { name: "twitter:description", content: BRAND.description },
        { name: "twitter:image", content: adminOgImage },
      ],
      link: [
        { rel: "icon", type: "image/svg+xml", href: brandAssetUrl(adminBaseURL, "flat") },
        { rel: "alternate icon", type: "image/x-icon", href: brandAssetUrl(adminBaseURL, "faviconRoot") },
        { rel: "apple-touch-icon", sizes: "180x180", href: brandAssetUrl(adminBaseURL, "apple") },
      ],
    },
  },
  modules: ["@pinia/nuxt", "@vueuse/nuxt", "@nuxtjs/tailwindcss", "@nuxt/eslint"],
  // 顺序有意义：tokens.css 定义全部颜色/排版 token，必须排在 admin.css 之前，
  // 否则 admin.css 里旧样式的 var(--bg-page) 之类的过渡层变量会拿不到值。
  css: ["~/assets/css/fonts.css", "~/assets/css/tokens.css", "~/assets/css/admin.css"],
  build: {
    transpile: ["naive-ui", "vueuc", "date-fns"],
  },
  vite: {
    optimizeDeps: {
      include: ["@tabler/icons-vue", "naive-ui"],
    },
    build: {
      rollupOptions: {
        onwarn(warning, defaultHandler) {
          if (isModulePreloadSourcemapWarning(warning.message)) return;
          defaultHandler(warning);
        },
        output: {
          manualChunks(id) {
            const moduleId = normalizeModuleId(id);
            if (
              naiveUiFoundationPackages.some((packageName) =>
                moduleId.includes(packageName),
              )
            ) {
              return "vendor-naive-foundation";
            }
          },
        },
      },
    },
    ssr: {
      noExternal: ["naive-ui", "vueuc", "date-fns"],
    },
  },
  runtimeConfig: {
    envProfile,
    apiUpstream,
    public: {
      envProfile,
      apiBase,
      siteUrl: adminSiteUrl,
      webSiteUrl:
        process.env.NUXT_PUBLIC_WEB_SITE_URL ||
        process.env.BLOGX_ADMIN_WEB_SITE_URL ||
        process.env.BLOGX_WEB_SITE_URL ||
        "http://localhost:3000",
    },
  },
  nitro: {
    devProxy,
  },
  routeRules: {
    "/**": { ssr: false },
  },
  devServer: {
    port: 3001,
  },
});
