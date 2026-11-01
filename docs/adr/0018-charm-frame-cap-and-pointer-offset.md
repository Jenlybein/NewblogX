# ADR-0018：光标挂件的帧率上限交给后台，取消自适应降频，另加可配的指针端偏移

- 状态：已交付
- 日期：2026-11-01
- 来源：设计对话（"鼠标停止移动后看起来像换了帧率、瞬间很卡"的实测诊断）· spec [#71](https://github.com/Jenlybein/NewblogX/issues/71) · 修订 [ADR-0016](./0016-deco-segment-and-cursor-charm.md) 的决策 9 / 已否决段 / 代价段第 4 条 / 当前生效约束 8

## 背景与问题

诊断结论来自对真实实现（`advanceCharmRope` / `charmTetherGeometry`）在两档采样率下的直接测量，不是观感推测：

1. **降频点落在绳子动得最凶的时刻**。ADR-0016 的车道闸门看的是"指针懒了多久"（`CHARM_POINTER_ACTIVE_WINDOW_MS = 100`），而不是"绳子还在不在动"。链的阻尼只有每步 0.99（120Hz 固定步长），停手后绳子要 1–2 秒才收敛；指针停住 100ms 就切进 20fps 车道，而此时尾端每 16.7ms 仍在走 17px（指针速度 1000px/s 时）。同一段物理运动被换成 50ms 采样后，每帧要跳 47px——这不是"省电"，是**把最需要帧的那一秒按 1/3 采样**，主观就是"帧率被改了"。
2. **`setTimeout(50)` 不对齐 vsync**。可见更新间隔在 50ms 与 66.7ms 之间抖动（定时器落在帧边界之后就要等下一次合成），即 15–20fps 的不均匀步进。不均匀比低帧率更容易被读成"卡"。
3. **运营没有任何帧率旋钮**。省电与顺滑都只能改代码发版，与 ADR-0016 决策 3（"参数做成后台可调，而不是硬编码"）的口径相悖；而实测这段的 CPU 成本很低（推进 + 几何 0.31ms/帧），真正贵的是**每帧重绘那个全屏 SVG 图层**——那正是该由运营权衡的量。
4. **绳头压在光标正中央**。快速拖行时视觉上像"绳子从指尖中间长出来"，希望有一个可配的极小偏移（右下 3px 量级）。

## 决策

1. **只保留一条调度：`requestAnimationFrame`**。删除 `CHARM_IDLE_FRAME_INTERVAL_MS`、`CHARM_POINTER_ACTIVE_WINDOW_MS`、`frameKind`、`lastPointerMoveAt`、`pointerActivelyDriving()`，以及 `useCursorCharm` 的 `setTimeout` / `clearTimeout` 注入面。任何"按运行时状态切换帧率"的闸门都不再存在。
2. **帧率上限进 `deco.charm.fps_cap`**：默认 `60`，合法区间 `30–160`，整数，单位 fps。语义是**最高渲染帧率**：高于显示器刷新率时等价于每帧（这样 160 档在 60Hz 屏上就等于"不限"，不需要再引入 `0 = 不限` 这种特殊值）。
3. **实现是"时间门控 + 早退"**：每个动画帧先比较 `now - lastPublishAt < 1000 / fps_cap - 4`，未到点**直接 `return`**（不推进物理、不发布）；到点才 `advanceCharmRope` + `publish`，并记 `lastPublishAt`。
   - 减 4ms 是容差，不是优化：60Hz 屏上一帧是 16.6667ms，`1000/60` 也是 16.6667ms，严格比较会因浮点与时间戳抖动**间歇性判成未到点**，把 60 档打成 30 档。
   - 高刷屏上实际帧率落在刷新率的整数分之一：144Hz + 90 档 = 每 2 帧 = 72fps（≤ 上限）。后台那个数字是上限，不是保证。
4. **物理常量一律不动**：1/120s 固定步长、0.05s 累加器上限、单帧最多 8 个子步（ADR-0016 约束 5）。下限取 `30` 正是因为 33.3ms 对 0.05s 留了 16.7ms 余量；`20` 档一帧恰好 50ms、余量为零，任何迟到都会被截断丢掉——那会让绳子**慢放**，比"少画几次"糟得多。
5. **指针移动不再自己发布**。`pointermove` 只写 `pointer` 坐标（首次移动把 `visible` 置真），唯一的发布口是门控后的那一帧，**没有任何例外**（首帧、`pointerover`、`focus` 都不放行）。拖动时最多有 `1000/fps_cap` 的随手延迟，60 档下 16.7ms，而挂件本身有 200ms 淡入，看不出来。
6. **保留三条"跑不跑"的机制**，它们不改变帧率：轻晃关闭且链收敛后停帧（`charmRopeSettled`）、`document.hidden` 时暂停、窗口失焦 / 指针离开文档时冻结指针端（绝不瞬移）。
7. **轻晃保留、默认仍开**（0.8Hz / 幅度 20）。恒定上限下"阅读时也在重绘"是显式接受的代价，省电档位交给运营：把 `idle_sway_amp` 设 0（收敛后回零帧）或把 `fps_cap` 调低。
8. **指针端偏移**：物理头节点 = 指针热点 + `(pointer_offset_x, pointer_offset_y)`，默认 `(3, 3)`，每轴合法区间 `-24–24`，单位 CSS px（与 `clientX/clientY` 同一视口坐标系，不随 DPI 缩放），允许负值（可往左上偏）。偏移只在这一处换算，渲染不得再单独偏一次。
9. **新增第四条钳位规则 `normalizeSignedRange`**（夹到 `[min, max]`、NaN/±Inf 回落默认、`0` 是普通值）。现有三条都表达不了"允许负数的区间"：`normalizePositiveRange` 把 `<= 0` 一律回落默认，`normalizeNonNegativeRange` 把 0 当合法值保留、负值仍被夹回 `min`（而这里的 `min` 就是负数，等于负偏移被静默吃掉）。
10. **跨端一致门禁要跟着升级**：`scripts/check-deco-range-parity.mjs` 的数值正则扩到支持负号（否则负的 min 会被跳过并报 `missing-value`）；参数总数不再写死，改成"四源键集合相等 + 锚点参数必须在场"（`rope_length`、`fps_cap`、`pointer_offset_x`、`pointer_offset_y`），既不再每加一个参数就改一串数字，又拦得住"某个参数从四源同时消失"。

## 理由与已否决方案

**为什么不是"自适应降频"的任何变体**：病灶不在 20fps 这个数字，而在**闸门与被降采样的东西错位**——闸门看输入（指针懒了），被采样的却是输出（绳子仍在甩）。任何继续按输入状态切换帧率的方案都会重新制造同一个错位；按能量切换则等于把"帧率"和物理实现细节焊死，运营无法解释也无力调优。

**为什么选"早退不算"（方案 2）而不是"每帧算、只在到点发布"（方案 1）**：中间帧推进出来的状态永远不会被画出来，方案 1 等于算了不画。已否决方案 1；它换来的"每帧工作量均匀"由下限 30 兜住（33.3ms 一帧、6 个子步以内，仍在 0.05s 截断的余量里）。

**已否决**：

- **保留车道、只把 20fps 改成 30fps**：错位没解决，只是把"卡一下"变成"卡小一点"。
- **用 `setTimeout` 做降频**：不对齐 vsync、间隔抖动，且让页面重绘节奏由定时器决定。
- **物理也跟着降频**：物理是固定步长按真实时间推进的，总子步数由时间决定、不因帧率改变；跟着降频省不到什么，却让"帧率"重新耦合到绳子手感上。
- **用 `0` 表示"不限"**：后台表单纯元数据驱动（`NInputNumber` 的 `min/max` 来自 `decoParams`），`min = 30` 时运营根本输入不了 0；而且钳位规则要为它开一条特殊分支。`160` 档已经承担了这个语义。
- **门控不带容差**：见决策 3，会让 60 档在 60Hz 屏上间歇性掉成 30 档。
- **`pointermove` 里保留即时发布**：拖动是最贵、事件率最高的路径，给它开例外等于"帧率上限"名不副实；那正是"按状态改变帧率"换了个位置又回来。
- **放宽物理截断（0.05s → 0.1s）以支持更低的帧率**：那要改 ADR-0016 约束 5 并重验"切后台不甩爆"，为几个不会有人选的档位不值得。
- **偏移做成单标量（固定 45°）或"距离 + 方向"**：前者将来只想横向偏就得改代码；后者参数膨胀，与 ADR-0016 批评过的思路同类。
- **偏移只挪渲染起点、物理仍以光标为头**：弧垂与张力都算在错的原点上，快速拉扯时起点与绳子会脱节。
- **按帧计数跳帧（`N = ceil(刷新率 / fps_cap)`）**：渲染永远均匀，但要先估刷新率，估错就整体偏一档（60Hz 估成 120Hz 直接变 30fps）。
- **给装饰引入端到端帧率实测**：那需要 Playwright 一类依赖，超出本仓库现有测试分层；门控与偏移由注入假时钟 + 假动画帧的单测锁定即可。

## 代价 / 不做什么

- **恒定上限意味着"只是读文章"时也按上限重绘**：默认 60 相比原来的 20fps 常驻是 3 倍重绘。这是**显式推翻 ADR-0016 的「已否决：永远开着的 rAF 轻晃」**的代价；补偿是运营可把 `idle_sway_amp` 设 0（收敛后回零帧）或下调 `fps_cap`。
- **30 以下不开放**：物理截断不动，低于 30 只会慢放。运营若填 29，会被钳到 30 而不是"生效"。
- **高刷屏上运营填的数字是上限不是保证**（144Hz + 90 档 = 72fps）；后台文案必须说清，否则会被读成"配置没生效"。
- **偏移到 ±24 的极端值时，绳头与光标热点明显脱节**，视觉上像"断线"。这是有意的上界，不再放大范围。
- **不做什么**：不做"跟随显示器刷新率"这类布尔参数；不做按可见性/交互状态的第二套帧率；不引入配置实时推送（读者站仍是刷新生效，与其余七段一致）；不把 `fps_cap` 做成按主题或按页面；不改 `z-index 9999`、`pointer-events: none`、SVG 覆盖层这些 ADR-0016 的渲染立场。

## 当前生效约束

1. 挂件只有一条调度：`requestAnimationFrame`。禁止引入 `setTimeout` / `setInterval` 驱动的帧，也禁止按"指针是否活跃""链是否收敛"以外的运行时状态改变发布频率。
2. 帧率上限只来自 `deco.charm.fps_cap`（默认 60、区间 30–160、整数）；不得在前端写死帧率，也不得新增第二个帧率参数。
3. 门控判定是 `now - lastPublishAt >= 1000 / fps_cap - 4`；那个 4ms 容差不许删（删了 60 档在 60Hz 屏上会掉成 30 档）。
4. 物理常量不动：1/120s 固定步长、0.05s 累加器上限、单帧最多 8 个子步。不得为支持更低的 `fps_cap` 放宽它们。
5. 唯一的发布口是门控帧：`pointermove` / `pointerover` / `focus` / 首帧都不得绕过，事件里只允许写 `pointer` 坐标与 `visible`（循环启动时 `lastPublishAt` 视为已到点——那是初值，不是例外）。
6. 停帧（轻晃关且收敛）、`hidden` 暂停、失焦冻结三条保留；删除任何一条都要另开 ADR。
7. 指针端 = 指针热点 + `(pointer_offset_x, pointer_offset_y)`；物理头节点必须用这个换算，渲染不得再单独偏一次。
8. 偏移每轴 `-24..24`、默认 `3/3`、单位 CSS px；钳位必须用 `normalizeSignedRange`，不得用 `normalizePositiveRange` / `normalizeNonNegativeRange` 顶替（它们会静默吃掉负值）。
9. `deco.charm` 每个数值参数的 min/max/default 必须在后端 Go、读者站、后台、默认种子四处逐项一致，由 `check-deco-range-parity.mjs` 守着；该门禁必须支持负值，并改用"四源键集合相等 + 锚点参数清单"而不是写死参数数量。
10. 契约不受影响（`/site/{name}` 的 data 是 `kernel.SiteConfigData = map[string]any`）；不得为这三个参数新增 swagger 注解、路由或 DTO。

## 相关代码与文档

- 配置与校验：[backend/conf/conf_deco.go](../../backend/conf/conf_deco.go)（常量三元组、结构体、四条钳位规则）· [backend/config/site_default_settings.yaml](../../backend/config/site_default_settings.yaml)（默认种子）· [配置分层](../architecture/configuration.md) §五
- 读者站：[frontend/app/utils/decoCharm.ts](../../frontend/app/utils/decoCharm.ts)（常量、`CharmRopeConfig`、clamp/normalize）· [frontend/app/composables/useCursorCharm.ts](../../frontend/app/composables/useCursorCharm.ts)（门控与发布）· [frontend/app/components/deco/CursorCharm.vue](../../frontend/app/components/deco/CursorCharm.vue)
- 后台：[frontend_admin/app/utils/decoParams.ts](../../frontend_admin/app/utils/decoParams.ts)（参数元数据，表单纯元数据驱动）· [frontend_admin/app/types/api.ts](../../frontend_admin/app/types/api.ts)
- 门禁：[scripts/check-deco-range-parity.mjs](../../scripts/check-deco-range-parity.mjs) · [frontend/test/gates/deco-range-parity.test.ts](../../frontend/test/gates/deco-range-parity.test.ts) · [backend/conf/testdata/deco-charm-normalization.json](../../backend/conf/testdata/deco-charm-normalization.json)
- 上游：[ADR-0016](./0016-deco-segment-and-cursor-charm.md)（本 ADR 取代其决策 9、已否决段的"永远开着的 rAF 轻晃"、代价段第 4 条、当前生效约束 8；其余条款继续生效）· [ADR-0012](./0012-neutral-glass-visual-system.md)（视觉与动效预算）
- 词汇：[GLOSSARY · 光标挂件 / 挂绳](../../GLOSSARY.md)——实施时必须给「挂绳」词条补一句"指针端 = 指针热点 + 指针端偏移"，否则该词条在落地后就是错的。

**落地顺序（已全部落地）**：① 本 ADR + ADR-0016 的后续修订 → ② 门禁脚本：负号支持 + 键集合派生 + 锚点清单 → ③ 后端：`conf_deco.go` 三组常量 / 三个字段 / `normalizeSignedRange` + yaml 默认种子 + 夹具用例 + Go 测试 → ④ 读者站：`decoCharm.ts`（常量、配置形状、四条钳位）+ `useCursorCharm.ts`（删掉自适应降速、加门控、指针端偏移）+ 单测改写 → ⑤ 后台：`decoParams.ts` 元数据与 clamp、`types/api.ts`、文案 → ⑥ 文档口径收尾：[configuration.md](../architecture/configuration.md) §五 的降速段与参数清单、yaml 里关于旧降速策略的注释、`CursorCharm.vue` 的成本注释、GLOSSARY「挂绳」、[docs/adr/README.md](./README.md) 的索引状态。

## 实施记录（2026-11-01）

决策与约束正文一字未改（本目录规则：已交付的 ADR 不改结论）。上面「落地顺序」的六步已由分票 #72–#75 全部落地、#76 收尾文档口径；下面记录落地时与决策文字的偏离、以及最终形态，供之后读本 ADR 的人对照代码。

1. **绘制几何整段拆到新文件 [`frontend/app/utils/charmTether.ts`](../../frontend/app/utils/charmTether.ts)**。计划里它留在 `decoCharm.ts`；实际落地时那个文件已经有 742 行，加完指针端偏移的相关代码会冲到约 864 行 —— 超过 `frontend/eslint.config.mjs` 给 `app/**/*.ts` 定的 `max-lines 800`（`error` 级，是 CI 门禁而不是告警）。于是把"链节点 → 带形路径"整段（`CharmTetherGeometry`、`charmTetherGeometry`、弧垂系数、中心线采样密度）搬到 `charmTether.ts`。依赖方向单向：`charmTether.ts` 只从 `decoCharm.ts` 取 `CHARM_TAPER_RATIO` / `clampCharmLineWidth` / `CharmPoint`，反向没有引用。**拆的是文件，不是行为**：发布路径仍是一次 `charmTetherGeometry` → 路径 `d` + 站标中心。
2. **"整数"的统一口径 = 先夹边界、再取整**（`fps_cap` 与 `pointer_offset_x` / `pointer_offset_y` 同一条顺序）。取整是"半值远离 0"：`-2.5 → -3`；读者站 / 后台写成 `Math.sign(v) * Math.round(Math.abs(v))`，与 Go 的 `math.Round` 对齐（直接写 `Math.round` 会在负半值上与 Go 分叉）。顺序也不能反：`24.6` 先夹到 `24` 再取整，反过来会得到 `25`。这条口径由共享夹具钉住（见第 7 条）。
3. **`0` 是合法偏移，不是缺陷**。偏移走 `normalizeSignedRange`（`0` 与负数都是普通值）：库里缺这两个键时钳出来就是 `0`，指针端正好压在指针热点上、等于关掉偏移；默认种子是 `3/3`，所以"新装即右下 3px"。想回到"压在热点上"就把两轴都填 0 —— 这与"填错被静默纠正"无关，是语义。
4. **帧率门控的最终形态**：容差落成常量 `CHARM_FRAME_GATE_TOLERANCE_MS = 4`（判定是 `now - lastPublishAt >= 1000 / fps_cap - 4`，即决策里的字面 `-4` 被命名了）；`lastPublishAt` 初值 `null` 视为"已到点"，循环一启动的第一帧就发布、不空等一个间隔（决策 3 / 约束 5 括号里那句"初值不是例外"）；`deltaSeconds` 取**距上一次发布的真实流逝时间**（`lastFrameTime` 只在发布那一帧更新），因此被门控挡掉的帧不会让绳子慢放 —— 30 档一帧仍推进 33.3ms，且仍由 0.05s 累加器上限截断。
5. **链在第一帧按当时的指针位置建立**。`pointermove` 只写 `pointer` 坐标与 `visible`，建链发生在门控帧的 `ensureRope(head, config)` 里（决策 5 / 约束 5 的直接后果）。连带影响是三条冻结 / 暂停用例的前提改了措辞：必须先跑一帧把链建起来、再跳指针，否则"冻结前后头节点不变"断言的是空链。这是**测试前提**的调整，不是行为妥协 —— 在事件里建链就等于给拖动开了发布口子。
6. **跨端门禁的最终形态**：四源**键集合相等**（哪一端缺参数或多参数都报红，并点名是哪两端）+ **锚点参数清单带交付状态位**（`delivered` / `pending`）。四个锚点（`rope_length` / `fps_cap` / `pointer_offset_x` / `pointer_offset_y`）现在全是 `delivered`。状态位机制**有意保留**给下一批参数：新参数可以先标 `pending`（四端都没有也不算红），实现那一笔提交再把状态改成 `delivered`；反过来，标了 `delivered` 却从四端同时消失就是失败 —— 这是"参数被静默删掉"唯一的拦截点。
7. **最终数字**：`deco.charm` = 总开关 `enable` + **11 个数值参数** —— `logo_size` / `rope_length` / `segments` / `line_width` / `gravity` / `damping` / `idle_sway_hz` / `idle_sway_amp`，加本 ADR 的三个新参数 `fps_cap`（默认 60、区间 30–160）、`pointer_offset_x` / `pointer_offset_y`（默认 3/3、每轴 -24–24）；共享归一化夹具 [deco-charm-normalization.json](../../backend/conf/testdata/deco-charm-normalization.json) **111 条用例**，后端 / 读者站 / 后台三套测试共同消费，与 [`scripts/check-deco-range-parity.mjs`](../../scripts/check-deco-range-parity.mjs) 的取值范围门禁不是同一道门（前者判取值、后者判四源一致）。
8. **术语统一（评审后收口）**：`GLOSSARY` 的唯一正名是「指针端」，实现里链的第 0 个节点称「指针端节点」，旧叫法「绳头」已进 `_Avoid_`。本 ADR 的决策 8 / 约束 7 沿用写作时的「物理头节点」，指的就是这个「指针端节点」；代码、配置注释、后台文案与 `configuration.md` 已统一到「指针端」。

**收尾校验（#76 实跑）**：`node scripts/check-deco-range-parity.mjs` 绿（派生出 11 个数值参数、111 条夹具被三套测试消费）；`frontend` 的 `test/gates` + `test/design` 与后台 `test/unit/settings-wording.test.ts` 全绿。文档口径同步见 [ADR-0016 后续修订](./0016-deco-segment-and-cursor-charm.md)、[ADR 索引](./README.md)、[配置分层 §五](../architecture/configuration.md)、[GLOSSARY · 挂绳](../../GLOSSARY.md)。
