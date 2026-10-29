// 检查模板里用到的自定义组件是否真的能解析到（两端前端共用）。
//
// 为什么需要它（真实事故）：`app/components/settings/AiProviderList.vue` 在
// Nuxt 里会被**带路径前缀**注册成 `SettingsAiProviderList`（目录名 Settings +
// 文件名 AiProviderList 不重复，前缀不会被去掉）。组件里写 `<AiProviderList />`
// 既不报编译错误、也不报类型错误，只在运行时 resolveComponent 失败 —— 于是
// 整块「模型配置」UI（含「添加配置」按钮）在线上直接不渲染，页面看着"正常"，
// 功能凭空消失，且 lint / vue-tsc / vitest 全绿，因为它们都看不到模板标签能不能解析。
//
// 判定规则：标签要么来自该 SFC 的 import 绑定，要么在 Nuxt 自动注册表
// （.nuxt/components.d.ts）里，要么是框架/浏览器内置的少数几个。
//
// 用法：node scripts/check-components.mjs [前端目录]   （目录需先跑过 nuxt prepare）
//       不传目录时用当前工作目录，方便在 frontend / frontend_admin 下直接跑。
// 退出码：0 = 全部可解析；1 = 有无法解析的标签（会把文件与标签都打出来）
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const repoRoot = fileURLToPath(new URL('..', import.meta.url))
const targetArg = process.argv[2]
const projectDir = targetArg ? resolve(process.cwd(), targetArg) : process.cwd()
const appDir = join(projectDir, 'app')
const componentsDts = join(projectDir, '.nuxt/components.d.ts')

// 框架/浏览器内置标签，不需要 import 也不在自动注册表里。
const BUILTIN = new Set([
  'NuxtLink',
  'NuxtPage',
  'NuxtLayout',
  'NuxtErrorBoundary',
  'NuxtLoadingIndicator',
  'NuxtRouteAnnouncer',
  'NuxtWelcome',
  'NuxtRoot',
  'ClientOnly',
  'DevOnly',
  'Transition',
  'TransitionGroup',
  'KeepAlive',
  'Teleport',
  'Suspense',
])

/** 收集 Nuxt 自动注册的组件名（含 Lazy 前缀变体）。 */
function loadAutoRegistered() {
  let source = ''
  try {
    source = readFileSync(componentsDts, 'utf8')
  } catch {
    console.error(`✗ 找不到 ${relative(repoRoot, componentsDts)}，请先在该前端目录执行 nuxt prepare`)
    process.exit(1)
  }
  const names = new Set()
  // export const SettingsAiProviderList: typeof import("../app/components/settings/AiProviderList.vue")['default']
  for (const match of source.matchAll(/export const (\w+): typeof import\("[^"]+"\)/gu)) {
    names.add(match[1])
  }
  // export const LazyAuthEmailLoginForm: LazyComponent<typeof import("...")['default']>
  // 懒加载变体同样是合法标签名（<LazyXxx />），漏了会误报。
  for (const match of source.matchAll(/export const (Lazy\w+): LazyComponent</gu)) {
    names.add(match[1])
  }
  return names
}

/** 递归列出 app 下所有 .vue 文件。 */
function listVueFiles(dir) {
  const out = []
  for (const name of readdirSync(dir)) {
    const full = join(dir, name)
    if (statSync(full).isDirectory()) {
      if (name === 'node_modules' || name === '.nuxt') continue
      out.push(...listVueFiles(full))
    } else if (name.endsWith('.vue')) {
      out.push(full)
    }
  }
  return out
}

/** 取出 SFC 里能用作模板标签的本地绑定名。 */
function importsOf(source) {
  const names = new Set()
  // 1) import 绑定的组件，覆盖多行 import，包括 `import type`。
  const re = /import\s+(?:type\s+)?([\s\S]*?)\s+from\s+['"][^'"]+['"]/gu
  for (const match of source.matchAll(re)) {
    const clause = match[1]
    // 默认导入：`Foo` 或 `Foo, { bar }`
    const defaultMatch = /^\s*([A-Za-z_$][\w$]*)\s*(?:,|$)/u.exec(clause)
    if (defaultMatch) names.add(defaultMatch[1])
    // 具名导入：`{ A, B as C }`
    const braceMatch = /\{([\s\S]*)\}/u.exec(clause)
    if (braceMatch) {
      for (const part of braceMatch[1].split(',')) {
        const piece = part.trim().replace(/^type\s+/u, '')
        if (!piece) continue
        const asMatch = /\bas\s+([A-Za-z_$][\w$]*)$/u.exec(piece)
        names.add(asMatch ? asMatch[1] : piece)
      }
    }
  }
  // 2) 变量声明的组件：`const X = defineAsyncComponent(...)`（读者站大量在用），
  //    它们同样能出现在模板里，且名字是任意的、跟实际文件不对应。
  for (const match of source.matchAll(/(?:const|let|var)\s+([A-Z][A-Za-z0-9_$]*)\s*=/gu)) {
    names.add(match[1])
  }
  return names
}

/** 取出模板里所有 PascalCase 标签名（只看 <template> 段，避免把 TS 泛型当标签）。 */
function templateTagsOf(source) {
  const tags = new Set()
  const templateMatch = /<template>([\s\S]*)<\/template>/u.exec(source)
  const scope = templateMatch ? templateMatch[1] : source
  const re = /<([A-Z][A-Za-z0-9]*)/gu
  for (const match of scope.matchAll(re)) {
    tags.add(match[1])
  }
  return tags
}

const autoRegistered = loadAutoRegistered()
const files = listVueFiles(appDir)
const problems = []

for (const file of files) {
  const source = readFileSync(file, 'utf8')
  const imported = importsOf(source)
  for (const tag of templateTagsOf(source)) {
    if (BUILTIN.has(tag) || imported.has(tag) || autoRegistered.has(tag)) continue
    problems.push({ file: relative(repoRoot, file), tag })
  }
}

if (problems.length > 0) {
  console.error('✗ 以下模板标签解析不到（运行时会被静默跳过，整块 UI 不渲染）：')
  for (const { file, tag } of problems) {
    console.error(`  ${file}: <${tag} />`)
  }
  console.error('')
  console.error('修法：在 <script setup> 里显式 import 这个组件。')
  console.error('注意 components/ 子目录会带路径前缀注册：settings/X.vue 的注册名是 SettingsX。')
  process.exit(1)
}

const appLabel = relative(repoRoot, projectDir) || projectDir
console.log(`✓ 组件解析检查通过（${appLabel}：${autoRegistered.size} 个自动注册组件，${files.length} 个 .vue 文件）`)
