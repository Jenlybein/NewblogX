import { existsSync, readFileSync  } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { defineNuxtConfig } from 'nuxt/config'

import { BRAND, brandAssetUrl, brandFullName } from './app/utils/brand'

const appRoot = dirname(fileURLToPath(import.meta.url))
const hydratedEnvKeys = new Set<string>()


// 只允许注入本应用命名空间下的键（以及 Nuxt 自身的 NUXT_ 键）。
// 历史原因：env 文件曾经从后端配置里抄过来，一旦有人复制了 BLOGX_JWT_SECRET
// 这类后端密钥，它就会被无差别写进前端进程环境。这里用白名单从源头堵住。
const ALLOWED_ENV_PREFIX = ['BLOGX_WEB_', 'NUXT_']
function isAllowedEnvKey(key: string) {
  return ALLOWED_ENV_PREFIX.some((prefix) => key.startsWith(prefix))
}

function hydrateProcessEnv(relativePath: string, options: { overrideHydrated?: boolean } = {}) {
  const envPath = resolve(appRoot, relativePath)
  if (!existsSync(envPath)) {
    return
  }

  const source = readFileSync(envPath, 'utf8')
  for (const rawLine of source.split(/\r?\n/u)) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) {
      continue
    }

    const separatorIndex = line.indexOf('=')
    if (separatorIndex <= 0) {
      continue
    }

    const key = line.slice(0, separatorIndex).trim()
    // 非本应用命名空间的键直接忽略（防止后端密钥泄漏到前端进程环境）
    if (key && !isAllowedEnvKey(key)) {
      continue
    }
    if (
      !key ||
      (process.env[key] !== undefined && !(options.overrideHydrated && hydratedEnvKeys.has(key)))
    ) {
      continue
    }

    let value = line.slice(separatorIndex + 1).trim()
    const isQuoted = (value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))
    if (isQuoted) {
      value = value.slice(1, -1)
    }

    process.env[key] = value
    hydratedEnvKeys.add(key)
  }
}

const envProfile =
  (process.env.NUXT_ENV_PROFILE || process.env.BLOGX_WEB_ENV_PROFILE || 'local-local').trim() || 'local-local'
hydrateProcessEnv('env/common.env')
hydrateProcessEnv(`env/${envProfile}.env`, { overrideHydrated: true })

function normalizeOrigin(value: string) {
  return value.endsWith('/') ? value.slice(0, -1) : value
}

function normalizePathSegments(value: string) {
  return value.replace(/^\/+|\/+$/gu, '').split('/').filter(Boolean)
}

function joinOriginPath(origin: string, path: string) {
  const targetUrl = new URL(normalizeOrigin(origin))
  const originSegments = normalizePathSegments(targetUrl.pathname)
  const pathSegments = normalizePathSegments(path)

  if (
    originSegments.length > 0 &&
    pathSegments.length > 0 &&
    originSegments[originSegments.length - 1] === pathSegments[0]
  ) {
    pathSegments.shift()
  }

  const targetPath = [...originSegments, ...pathSegments].join('/')
  targetUrl.pathname = targetPath ? `/${targetPath}` : '/'
  return normalizeOrigin(targetUrl.toString())
}

function normalizeModuleId(value: string) {
  return value.replace(/\\/gu, '/')
}

function isModulePreloadSourcemapWarning(message: string) {
  return (
    message.includes('[plugin nuxt:module-preload-polyfill]') &&
    message.includes('Sourcemap is likely to be incorrect')
  )
}

const naiveUiFoundationPackages = [
  '/node_modules/vueuc/',
  '/node_modules/seemly/',
  '/node_modules/evtd/',
  '/node_modules/vdirs/',
  '/node_modules/@css-render/',
  '/node_modules/css-render/',
]

const apiUpstream = normalizeOrigin(
  process.env.NUXT_API_UPSTREAM ||
    process.env.BLOGX_WEB_API_UPSTREAM ||
    process.env.NUXT_API_ORIGIN ||
    process.env.NUXT_PUBLIC_API_BASE ||
    'http://127.0.0.1:8080',
)

const siteUrl = normalizeOrigin(process.env.NUXT_PUBLIC_SITE_URL || process.env.BLOGX_WEB_SITE_URL || 'http://localhost:3000')
const apiBase = String(process.env.NUXT_PUBLIC_API_BASE || process.env.BLOGX_WEB_API_BASE || '/api').trim() || '/api'
const wsPath = String(process.env.NUXT_PUBLIC_WS_PATH || process.env.BLOGX_WEB_WS_PATH || '/api/chat/ws').trim() || '/api/chat/ws'
// 资源基址默认 /uploads：后端只把 uploads 目录挂成静态（router/enter.go 的
// r.Static("/uploads", "./uploads")），没有"根路径静态"。以前默认 /_origin
// 是靠 nginx 把 /_origin/* 代理到后端根目录来实现的，属于历史别名。
const assetProxyBase =
  String(process.env.NUXT_PUBLIC_ASSET_PROXY_BASE || process.env.BLOGX_WEB_ASSET_PROXY_BASE || '/uploads').trim() ||
  '/uploads'
const apiProxyTarget = joinOriginPath(apiUpstream, apiBase)
const devProxy = apiBase.startsWith('/')
  ? {
      [apiBase]: {
        target: apiProxyTarget,
        changeOrigin: true,
        ws: true,
      },
    }
  : {}

// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: '2025-07-15',
  telemetry: false,
  devtools: { enabled: true },
  // 品牌页头/图标在这里给全站兜底；OG / twitter 的绝对地址需要运行时 siteUrl，
  // 放在 app.vue 的 useHead（见 app/utils/brand）。页面标题由 app.vue 的
  // titleTemplate 统一补品牌后缀。
  app: {
    head: {
      htmlAttrs: { lang: 'zh-CN' },
      title: brandFullName(),
      meta: [{ name: 'description', content: BRAND.description }],
      link: [
        { rel: 'icon', type: 'image/svg+xml', href: brandAssetUrl('/', 'flat') },
        { rel: 'alternate icon', type: 'image/x-icon', href: brandAssetUrl('/', 'faviconRoot') },
        { rel: 'apple-touch-icon', sizes: '180x180', href: brandAssetUrl('/', 'apple') },
      ],
    },
    // 路由切换过渡：out-in 保证同一时刻文档流里只有一页（不会两页并排导致滚动条/高度跳变）。
    // 过渡样式在 app/assets/css/tailwind.css 里（只动 opacity/transform，时长取自动效 token）。
    pageTransition: { name: "page", mode: "out-in" },
  },
  alias: {
    '#markdown-it': resolve(appRoot, 'node_modules/markdown-it/index.mjs'),
    '#markdown-it-ins': resolve(appRoot, 'node_modules/markdown-it-ins/index.mjs'),
  },
  modules: ["@pinia/nuxt", "@vueuse/nuxt", "@nuxtjs/tailwindcss", "@nuxt/eslint", "motion-v/nuxt"],
  // 顺序有意义：tokens.css 定义全部颜色/排版 token，必须排在 tailwind.css 之前，
  // 否则旧样式的 var(--bg-page) 之类的过渡层变量会拿不到值。
  css: ["~/assets/css/fonts.css", "~/assets/css/tokens.css", "~/assets/css/tailwind.css"],
  build: {
    transpile: ["naive-ui", "vueuc", "date-fns"],
  },
  vite: {
    optimizeDeps: {
      include: [
        "@vee-validate/zod",
        "vee-validate",
        "zod",
        "@tabler/icons-vue",
        "markdown-it/index.mjs",
        "markdown-it-katex",
        "markdown-it-ins/index.mjs",
        "highlight.js",
      ],
    },
    build: {
      rollupOptions: {
        onwarn(warning, defaultHandler) {
          if (isModulePreloadSourcemapWarning(warning.message)) {
            return
          }

          defaultHandler(warning)
        },
        output: {
          manualChunks(id) {
            const moduleId = normalizeModuleId(id)

            if (naiveUiFoundationPackages.some(packageName => moduleId.includes(packageName))) {
              return 'vendor-naive-foundation'
            }

            if (moduleId.includes('/node_modules/lodash-es/')) {
              return 'vendor-lodash-es'
            }

            if (moduleId.includes('/node_modules/cytoscape-cose-bilkent/')) {
              return 'vendor-cytoscape-cose-bilkent'
            }

            if (moduleId.includes('/node_modules/cytoscape-fcose/')) {
              return 'vendor-cytoscape-fcose'
            }

            if (moduleId.includes('/node_modules/cytoscape/')) {
              return 'vendor-cytoscape'
            }
          },
        },
      },
    },
    ssr: {
      // motion-v（含 framer-motion/motion-dom）是纯 ESM，生产构建时若走 external，
      // Node 侧会按 CJS 去 require 而失败 —— 必须让 Vite 把它打进 SSR bundle。
      noExternal: ["naive-ui", "vueuc", "date-fns", "motion-v"],
    },
  },
  runtimeConfig: {
    envProfile,
    apiUpstream,
    public: {
      envProfile,
      siteUrl,
      apiBase,
      wsPath,
      assetProxyBase,
      uploadsBase: "/uploads",
    },
  },
  nitro: {
    experimental: {
      websocket: true,
    },
    devProxy,
  },
  // 渲染边界是**白名单**（ADR-0019 决策 1 / 约束 1），不是"按页面随手加"：
  // 服务端渲染只留给 `/` 与 `/article/**`（分享卡片与搜索引擎靠它）；列表页
  // 与个人主页的抓取价值不足以抵消每次直达的服务端渲染代价，搜索页与创作区
  // 维持 SPA。判定与"非白名单路由开 SSR 会红"的门禁在
  // test/gates/rendering-boundary.test.ts —— 增删这条规则前先改 ADR。
  routeRules: {
    "/articles": { ssr: false },
    "/users/**": { ssr: false },
    "/search": { ssr: false },
    "/studio/**": { ssr: false },
  },
  devServer: {
    port: 3000,
  },
})
