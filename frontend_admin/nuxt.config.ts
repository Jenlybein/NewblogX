import { existsSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { defineNuxtConfig } from "nuxt/config";

const appRoot = dirname(fileURLToPath(import.meta.url));
const hydratedEnvKeys = new Set<string>();

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
  app: {
    baseURL: adminBaseURL,
  },
  modules: ["@pinia/nuxt", "@vueuse/nuxt", "@nuxtjs/tailwindcss"],
  css: ["~/assets/css/fonts.css", "~/assets/css/admin.css"],
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
      siteUrl:
        process.env.NUXT_PUBLIC_SITE_URL ||
        process.env.BLOGX_ADMIN_SITE_URL ||
        "http://localhost:3001/admin",
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
