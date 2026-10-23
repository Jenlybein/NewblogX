import { existsSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { readFileSync } from 'node:fs'
import { defineNuxtConfig } from 'nuxt/config'

const appRoot = dirname(fileURLToPath(import.meta.url))
const hydratedEnvKeys = new Set<string>()

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
const assetProxyBase =
  String(process.env.NUXT_PUBLIC_ASSET_PROXY_BASE || process.env.BLOGX_WEB_ASSET_PROXY_BASE || '/_origin').trim() ||
  '/_origin'
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
  alias: {
    '#markdown-it': resolve(appRoot, 'node_modules/markdown-it/index.mjs'),
    '#markdown-it-ins': resolve(appRoot, 'node_modules/markdown-it-ins/index.mjs'),
  },
  modules: ["@pinia/nuxt", "@vueuse/nuxt", "@nuxtjs/tailwindcss"],
  css: ["~/assets/css/fonts.css", "~/assets/css/tailwind.css"],
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
      noExternal: ["naive-ui", "vueuc", "date-fns"],
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
  routeRules: {
    "/search": { ssr: false },
    "/studio/**": { ssr: false },
  },
  devServer: {
    port: 3000,
  },
})
