# 读者站性能实测（2026-11-01）

> 本文只放**测量事实**，不放决策。决策见 [ADR-0019](../adr/0019-reader-rendering-and-cache-strategy.md)，跟踪票见 [#79](https://github.com/Jenlybein/NewblogX/issues/79)。
> 用途：改前基线；改后用同一脚本复测对比。

## 测法

- 站点：生产环境 `https://gentlybeing.cn`（单机 3.6 GB 内存，腾讯云）。
- 工具：Playwright + Chromium，本机（与站点同处国内网络）。冷 = 全新上下文；热 = 复用已访问上下文。
- 每个场景 3 次，下表为**中位/最差**。
- 场景：A 直达首页；B 首页点击第一篇文章；B2 直达文章页；C 从文章页浏览器返回；D 首页点击搜索入口；D2 直达 `/search`；E 直达后台登录页。
- 另在 1440 / 1024 / 768 / 390 视口、`prefers-reduced-motion`、headed 与 headless 两种 Chrome 下各复测一次。

## ⚠️ 首要发现：站内客户端跳转是坏的，不是慢

点击站内任意链接后：

- **+24 ms** URL 已变成目标路由；
- **目标视图从未插入 DOM**（MutationObserver 4 秒内 0 次插入，15 秒后仍不出现，页面停在原页）；
- **+225 ms** 抛未捕获异常：

```
TypeError: n.off is not a function
    at .../_nuxt/Bu_Yx5HU.js:1:7874        ← 源码 useParallax.ts 的 dispose
    at .../_nuxt/CMQphd2a.js:1:4540        ← Vue effectScope.stop()（路由切换清理）
```

出错源码：`function i(){n.off("change",c)}return re()&&oe(i),{offset:s,parallaxStyle:l,dispose:i}` ——
滚动视差组合式在路由切换、组件卸载的 `dispose` 里调用了 **MotionValue 上并不存在的 `off` 方法**（motion-v / framer-motion 的正确用法是 `on()` 返回的取消订阅函数）。文章、搜索、文章页的"回首页"链接全部复现。

**影响**：`contentVisible` 在 B、D 两个场景只能记"**从不出现（≥15 s）**"，无法测毫秒数；"切页慢"的用户体感里，很大一部分其实是**点击后页面不出现**。
**来源**：该调用自提交 `2bd7931`（#23 液态玻璃层 + 滚动进场 + 视差）引入，motion-v 版本未变——属于**长期存在的缺陷**，不是最近部署引入。首页另有 `Hydration completed but contains mismatches` 警告。

## 首屏（文档级加载）

| 场景 | 首字节 | DOM 就绪 | 内容可见 | 总下载 | 请求数 |
| --- | --- | --- | --- | --- | --- |
| A 首页（冷） | 225 / 387 ms | 891 / 1082 ms | **1644 / 2097 ms** | 2.9 MB | 66 |
| A 首页（热） | 114 / 119 ms | 143 / 149 ms | **297 / 299 ms** | — | 66 |
| B2 文章页（冷） | 219 / 482 ms | 616 / 955 ms | 1572 / 2211 ms | 1.8 MB | 68 |
| D2 `/search`（冷） | 166 / 177 ms | 456 / 470 ms | 1257 / 2485 ms | 3.0 MB | 77 |
| E 后台登录页（冷） | 233 / 339 ms | 546 / 734 ms | 902 / 1111 ms | 2.7 MB | 30 |

**首屏子资源构成（首页冷启动，总计约 2,909 KiB）**：

| 项 | 字节 | 占比 |
| --- | --- | --- |
| **Noto Sans SC 两个整包字体** | **2,314,796**（700 字重 1,172,244 + 400 字重 1,142,552） | **77.7%** |
| Sora / Inter 字体（4 个文件） | 合计 2,407,904（含上面两个 Noto） | — |
| JS | 463,311 | ~16% |
| CSS / 图片 | 其余 | — |

传输层：全站 **HTTP/1.1**，同域 6 个并发连接；首屏字体与 CSS/JS 从 228 ms 起排队。
首屏 TTFB 分解（冷启动第 1 次）：DNS 5 ms + TCP 112 ms + TLS 110 ms + 服务端 103 ms。

## 点击导航期间的取数（崩溃前实测到的部分）

- 首页 → 文章：**5 个 `/api/*`，全部 `initiatorType = fetch`**，期间 **0 个文档请求、0 个 `_payload`** —— 客户端导航是浏览器直接打 API，SSR 成果没有被复用。
- 时序**串行**：`articles/{id}` → `author_info` → `comments` → `users/base` 首尾相接（+7 → +63 → +116 → +176 ms），第 5 个 `ai/scoring` 与第 4 个并行；墙钟 271–348 ms。
- 同一文章 URL 用**文档导航**只发 3 个 API，且没有 `/api/articles/{id}`——HTML 里直接含 `<h1>` 与正文（72,917 B）。**SSR 只对直接打开/刷新生效，对站内点击不生效。**
- 点击期间下载的 JS = **0 B**（路由入口 chunk 首屏已加载），所以"第一次进搜索页要下整个搜索页 chunk"的预期不成立。
- `/search` 是 `data-ssr="false"` 的 1,419 B 空壳，加载后先并发 5 个 + 串行 1 个接口（合计 336 ms）才渲染结果。
- "浏览器返回"是**文档级重载**（`timeOrigin` 改变、重跑 66 个请求），不是 bfcache。

## 与后台的对照（为什么后台"切页快"）

| 维度 | 读者站 | 后台 |
| --- | --- | --- |
| 渲染模式 | 首页/文章页 SSR；搜索/创作区 SPA | 全站 SPA |
| 首屏 HTML | 62 KB（SSR 全页） | 2.3 KB（空壳） |
| `modulepreload` 数量 | 39 | 2 |
| 单页数据请求 | 首页 3 个串行、文章页 4 串行 + 1 | 1–2 个 |
| 跨导航缓存 | 无（`usePagedResourceCache` 初始化即 `force: true`） | 不需要（切页不离开文档） |

后台快不是优化得好，而是它的模型里没有"切页重新渲染文档 + 重新取数"。**但它同样是 Nuxt 应用，同样加载了 2.5 MB 字体**——只是它切页不重载文档，所以你感觉不到。

## 测量注意事项

1. 崩溃场景（B/D）无法给出毫秒数，只能记"从不出现"；修复后才能拿到真实的切页耗时基线。
2. 本机网络质量好、与站点同处国内；移动网络下"串行接口"与"2.4 MB 字体"的影响会放大。
3. 早期单轮探测（未复测）曾给出首页 1358 ms、文章页 1209 ms，与本文中位数差异来自网络抖动与运行次数；以本文（3 次中位/最差）为准。
4. 未做任何写操作；文章页自身的 `POST /api/articles/view`（阅读计数）是页面自动行为。

## 改后复测（2026-11-01）

> 上方全部改前基线原样保留、数字不动；本节是收尾票 #87 用**同一脚本、同一口径**的改后复测与补充验收。决策见 [ADR-0019](../adr/0019-reader-rendering-and-cache-strategy.md)，脚本口径见 [scripts/perf/README](../../scripts/perf/README.md)。

### 测法

- 站点：**本地生产构建**——`cd frontend && pnpm build`，然后 `NUXT_API_UPSTREAM=https://gentlybeing.cn node .output/server/index.mjs`（`127.0.0.1:3000`）。API 上游仍是生产（`https://gentlybeing.cn`）。生产站此时仍是旧构建，改后无法在生产上直接测。
- 改前补跑了一份**同一脚本的本地构建**（`main` `5530bb8`，`127.0.0.1:3001`）作同口径对照；生产基线（本文上方）作第二参照。
- 工具口径不变：Playwright + Chromium 156，1440×900，冷 = 全新上下文 + CDP 禁用 HTTP 缓存，热 = 同上下文先访问一次再测；每场景每模式 3 次，下表为**中位 / 最差**。
- 命令：
  ```bash
  cd frontend && pnpm build
  cd scripts/perf
  node measure.mjs --base http://127.0.0.1:3000 --label 2026-11-01-post-local --out baselines/2026-11-01-post-local
  node measure.mjs --base http://127.0.0.1:3001 --label 2026-11-01-pre-local  --out baselines/2026-11-01-pre-local
  ```
- 报告存档：[改后 `2026-11-01-post-local`](../../scripts/perf/baselines/2026-11-01-post-local/report.md) · [改前本地构建 `2026-11-01-pre-local`](../../scripts/perf/baselines/2026-11-01-pre-local/report.md) · [改前生产站 `2026-11-01-pre-p0`](../../scripts/perf/baselines/2026-11-01-pre-p0/report.md)。

### 四项硬指标（改后）

| 指标 | 改前·本地 main 5530bb8 | 改后·集成分支 + 过渡 120 ms | 预算 | 结论 |
| --- | --- | --- | --- | --- |
| 首屏内容可见 · 首页冷 | 515 / 962 ms | **314 / 435 ms** | < 800 ms | PASS |
| 首屏内容可见 · 首页热 | 378 / 440 ms | **161 / 172 ms** | < 800 ms | PASS |
| 首屏内容可见 · 文章页冷 | 583 / 1375 ms | **298 / 592 ms** | < 800 ms | PASS |
| 首屏内容可见 · 文章页热 | 446 / 448 ms | **167 / 184 ms** | < 800 ms | PASS |
| 切页内容可见 · 首页→文章冷 | 不可测（从不出现） | **226 / 230 ms** | P95 < 300 ms（3 次最差作保守代理） | PASS |
| 切页内容可见 · 首页→文章热 | 不可测（从不出现） | **232 / 233 ms** | 同上 | PASS |
| 首屏字体下载 · 首页冷 | 2,407,904 B | **93,108 B**（中文 0 B） | < 102,400 B | PASS |
| 返回上一页命中缓存（数据请求数 / 耗时） | 未命中（3 请求 / 194 ms，冷） | **命中（0 请求 / 0 ms，SPA 返回）** | 0 请求或 < 50 ms | PASS |

参考：首页冷启动总下载 3,941,461 → **1,634,564 B**（请求数 66 → 70；请求数略增来自渲染边界拆分后的 chunk 组织，不影响四项指标）。

### 过渡开销量化（切页为什么曾贴着 300 ms）

改后第一版仍沿用 `--motion-base`（200 ms）+ `mode: 'out-in'`（存档 [2026-11-01-post-local-200ms](../../scripts/perf/baselines/2026-11-01-post-local-200ms/report.md)），切页内容可见为 302 / 305 ms（冷）、299 / 308 ms（热），正好贴着预算。用探针（[transition-probe.mjs](../../scripts/perf/probes/transition-probe.mjs)，默认不进 CI）在同一构建上把 `.page-enter-active/.page-leave-active` 的过渡时长临时改 0，对比同一点击路径：

| 冷/热 | 点击→URL | 点击→骨架 | 点击→正文 h1 |
| --- | --- | --- | --- |
| 冷 · 过渡 200 ms | 29 ms | 骨架被离场挡住/不可见 | 291–318 ms |
| 冷 · 过渡关闭 | 21 ms | 87 ms | 120–171 ms |
| 热 · 过渡 200 ms | 32 ms | — | 291–315 ms |
| 热 · 过渡关闭 | 26 ms | — | 120–146 ms |

差值 ≈ **170–180 ms ≈ 离场时长**：`out-in` 下旧页必须完整离场才挂新页，取数因此被串行化在过渡之后，但取数本身只占 ~120–170 ms。因此按 ADR-0012 的三档把路由过渡降到 **fast 档 120 ms**（`tailwind.css` 由 `var(--motion-base)` 改 `var(--motion-fast)`；档位仍在 120 / 200 / 320 内，不改 `out-in`、只动 `opacity`/`transform`），同步改写 `frontend/test/design/motion-foundation.test.ts` 的断言（不是删除）。下调后切页 226 / 230（冷）、232 / 233（热），达标。理由与回退方式见 ADR-0019「实施记录」。

### 返回是否命中缓存（口径修订）

改前的"返回"用 `page.goto(文章页)` + 浏览器返回，必然产生文档级重载，测不到 30 s 客户端 SWR；这个口径还把"测不到"记成了"未命中"。#87 把场景改成**真实用户路径**：首页点击文章（客户端跳转）→ 浏览器返回。改后实测 `performance.timeOrigin` 不变、**0 个数据请求、0 ms**，命中客户端缓存；改前本地构建同一路径为 **3 个数据请求 / 194–210 ms**（旧的 `usePagedResourceCache` 初始化即 `force: true`，返回首页重新走三个接口）。P0 未修时站内点击进不去文章页，脚本保留"进文章退化为文档级 `goto`"的兜底并在报告标注（`entryFlow=document`），所以改前那行只作对照。详见 [scripts/perf/README](../../scripts/perf/README.md) 的口径说明。

### 匿名 SSR 缓存（补充证据，curl）

本地构建上对同一 URL 连发两次（`x-ssr-cache` 头为判据）：

| 目标 | 第一次（MISS）TTFB | 第二次（HIT）TTFB |
| --- | --- | --- |
| 首页 `/` | 200.5 ms | 1.5 ms |
| 文章页 `/article/374988074683207680` | 221.3 ms | 1.9 ms |

与 #86 的自测（首页 107.3 → 2.1 ms、文章 735.9 → 2.8 ms）同一结论，差异来自本机与上游当时的响应时间。缓存是进程内内存表、TTL 30 s、键只含 `pathname + search`。

### 分享卡片（抓取器 UA、无 JS）

本地构建上分别用 `facebookexternalhit/1.1` 与 `Baiduspider/2.0` 的 UA 请求文章页（无 JS，就是抓取器能看到的内容）：两次都拿到 72,315 B 的 SSR HTML，`og:title` / `og:description` / `og:image` / `<meta name="description">` / `<title>` / 正文 `<h1>` / 摘要 / 正文区块全部在 HTML 里。可复跑命令：

```bash
curl -s -A "facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)" \
  http://127.0.0.1:3000/article/<文章id> | grep -Eo 'og:title|og:description|<title>|article-hero__abstract'
```

真实微信/QQ 抓取无法在本机重现（它们会带自己的 UA 与服务端 IP），**人工待验**：把文章链接发到微信/QQ 对话框看预览卡片，或部署后用对应 UA 抓一次。本轮"无 JS 抓取器能看到标题/摘要/正文/og 标签"这一半已由上面的命令验证。

### 遗留观察

1. **hydration mismatch 仍存在，但只在首页**：Playwright 打开 `/` 会输出 `Hydration completed but contains mismatches.`，文章页没有。spec #79 已把它登记为独立观察项（Out of Scope）；P0 修复与本次改造后仍在，**建议另开票**定位（与本轮性能改动无关）。
2. `Home` 热启动有一次 733 ms 的抖动（早期轮次），最终复测热为 161–172 ms；对比时用中位/最差，不要拿单次极端值做结论。
3. 无头 Chromium 的"返回"只有在真实用户路径下才测得到客户端缓存；若用 `page.goto` 模拟，必然退化为文档级重载（见上）。
4. 30 s 内容滞后、登录态"先默认后跳变"、骨架观感、SSR 缓存重启即空/多实例不共享，都是 ADR-0019「代价 / 不做什么」里显式接受的代价。

## 原始产物

脚本与逐次原始数据（每次的 API、chunk、字体明细与请求瀑布）在仓外临时目录 `%TEMP%\blogx-perf\`（`results.json`、`report.md`）。实施阶段会把脚本整理进仓库，作为改前/改后对比的固定口径。
