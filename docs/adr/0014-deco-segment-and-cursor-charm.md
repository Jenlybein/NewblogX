# ADR-0014：装饰配置段与光标挂件（第 8 段 `deco` + 挂绳物理）

- 状态：已交付
- 日期：2026-10-31
- 来源：spec [issue #35](https://github.com/Jenlybein/NewblogX/issues/35)（开源实现调研与关键纠正是该 spec Further Notes 的一部分）· 视觉约束上游 [ADR-0011](./0011-neutral-glass-visual-system.md)

## 背景与问题

读者站目前只有一个"活的东西"——右下角的**捏捏**（[ADR-0010](./0010-jelly-replaces-traditional-ai-entry.md)）。但捏捏承载的是功能语义（读者站 AI 的唯一入口），任何能点的东西都会被读成功能而不是性格，所以它不能用来做纯装饰。站点站标（与 favicon 同一张画）在页头之外、与页面本身几乎没有联系。

管理侧同时有两个问题：这类装饰一旦做进前端，视觉参数就硬编码在代码里——想调挂绳长度、摆动速度或临时全站关掉，都得改代码发版；而原有承载业务语义的运行时配置段里没有"纯装饰"的容身处。

技术上有三处容易被"直觉"带错的地方：

- **"重力 = 下垂"是错的**。指针端被钉死的链，静止姿态是一条竖直直线；重力只塑造摆动瞬态。可见的弧线必须来自"绳长大于两端实际距离"。
- **光标挂绳的开源实现普遍帧率相关**。调研里被抄得最多的两个实现（`tholman/cursor-effects` 的弹簧链与 `subprotocol/verlet-js`）都硬编码每帧步长，在 120Hz 屏上物理会快一倍。这是真 bug，不能复制。
- 渲染方式、层级与"静止时要不要动"互相牵制：装饰要压在一切之上但不能挡住操作；指针停下后要有生命感，但帧率不能跟着指针状态来回切换，也不能把帧率与物理实现细节焊死。

此外，帧率若要交给运营权衡，就必须有一个后台可调的上限；而指针端压在光标正中央时，快速拖行看起来像"绳子从指尖中间长出来"，需要一个可配的极小偏移。

## 决策

1. **装饰配置只放在 `deco` 段，挂件参数只在 `deco.charm` 子块**。不把挂件塞进 `site` 或其余业务段：装饰会越来越多（背景、边框），而每开一个新段都要走一遍段注册的全部接点；独立段为"装饰"这一类一次付清成本，将来加装饰只加子块，不再开新段。`deco` 段本身由配置段注册表登记，注册表里不另抄参数。
2. **参数事实只有一张仓库级参数表**。`deco.charm` 的 12 个数值参数（含 `fps_cap` 与两轴指针端偏移）由 `config/deco-charm-parameters.json` 定义，另有布尔总开关 `enable`；构建期生成后端常量与归一化表、默认种子、读者站常量/参数清单、后台常量/元数据四端消费物，并把 min/max/default 注入契约 `conf.Charm`。标签、单位、提示等表单文案也留在参数表里由后台生成物消费。参数走运行时配置段的具名读写入口（`GET /site/admin/deco` / `PUT /site/deco` / `GET /site/deco`），两端字段类型由各自生成的 `openapi.d.ts` 派生，不手写视图模型，也不为参数新增专用 swagger 路由或自由键值 DTO。
3. **参数做成后台可调，而不是硬编码**：挂绳长度、节点数、描边宽度、站标尺寸、重力、阻尼、约束刚度、静止摆动频率与幅度、帧率上限、指针端偏移都进配置。理由：这些是"开局的合理值，不是最终审美"，硬编码会让调参必须发版，也没有临时全站关闭的手段。
4. **校验是"钳位、永不报错"**。写入路径上的归一化把越界值钳到边界、缺省补默认、NaN/非法回落默认；读者站与后台再各自钳一次作为最后一道防线。理由：管理员改一半被 400 打回只会让他猜哪里错了；而钳位能保证库里永远存不下发散值——例如阻尼大于 1 会让摆动每步放大、越甩越大。后台数值输入是「滑块 + 数字框」双输入、读写同一字段，越界/非法输入按同一钳位规则夹回并给行内可见提示。
5. **物理是 Verlet 链 + 由松弛驱动的绘制弧垂**。指针端由**带阻尼跟随**驱动（一阶低通：时间常数由 `stiffness` 换算、滞后有硬上界、静止时精确吸附），`advanceCharmRope` 在固定子步之间对锚点线性插值；其余节点自由，距离约束松弛若干次——这是"被一根绳子拖着"的惯性与滞后来源。**弧垂只由 `rope_length` 大于指针端到站标的实际距离产生**，描边沿节点走平滑曲线；调大重力只会让摆动更快，不会让静止的绳子弯下来。
6. **固定 1/120s 步长 + 时间累加器**（累加器上限 0.05s、单帧最多 8 个子步）。物理时间只由真实流逝时间决定，60Hz 与 120Hz 屏幕表现一致；掉帧、切后台、断点回来都不会让链被一帧甩爆。明确拒绝"每帧一步"的参考实现写法。
7. **渲染用单个全屏 SVG 覆盖层**，不用 canvas。理由是颜色能直接消费语义色 token（canvas 得每帧或切主题时手读计算样式，还会撞设计 token 门禁），亮/暗主题与高 DPI 全部免费，且测试环境没有 canvas 实现——选 canvas 等于给这段测试现写一个假引擎。绘制几何整段在 `charmTether.ts`，发布路径仍是一次 `charmTetherGeometry` → 路径 `d` + 站标中心。
8. **层级 `z-index: 9999`，压过弹窗与提示条**。这是有意选的"高于一切"：它会在 AI 面板、登录弹窗、toast 之上飘。安全性来自整层 `pointer-events: none` + `aria-hidden="true"`——"在上面"不等于"挡住"：点击穿透、不进 Tab 序列、读屏读不到。同时明确**不与捏捏避让**：绳子穿过它的一瞬是装饰的自然行为，避让反而会显出"它在躲人"。
9. **挂件只有一条 `requestAnimationFrame` 调度**。不存在 20fps 怠速车道，也不存在"指针移动期间走满速"：任何按运行时状态切换发布频率的闸门都不再有。帧率上限**只**来自后台 `deco.charm.fps_cap`（默认 `60`、区间 `30–160`、整数、单位 fps，语义是最高渲染帧率；高于显示器刷新率时等价于每帧，160 档在 60Hz 屏上就等于"不限"，不需要 `0 = 不限` 这种特殊值）。高刷屏上实际帧率落在刷新率的整数分之一，运营填的数字是**上限不是保证**，后台文案必须说清。
10. **实现是"时间门控 + 早退"**：每个动画帧先比较 `now - lastPublishAt >= 1000 / fps_cap - 4`，未到点**直接 `return`**（不推进物理、不发布）；到点才 `advanceCharmRope` + `publish`，并记 `lastPublishAt`。减 4ms 是容差，不是优化：60Hz 屏上一帧是 16.6667ms，`1000/60` 也是 16.6667ms，严格比较会因浮点与时间戳抖动间歇性判成未到点，把 60 档打成 30 档。`lastPublishAt` 初值视为已到点（那是初值，不是例外）；`deltaSeconds` 取距上一次发布的真实流逝时间，被门控挡掉的帧不会让绳子慢放。下限取 `30` 是因为 33.3ms 对 0.05s 累加器上限留了 16.7ms 余量；低于 30 只会把物理截断成慢放。
11. **唯一的发布口是门控帧**。`pointermove` 只写 `pointer` 坐标（首次移动把 `visible` 置真），`pointerover` / `focus` / 首帧都不得绕过门控发布，事件里只允许写坐标与 `visible`；链在第一帧按当时的指针位置由 `ensureRope` 建立。拖动时最多有 `1000/fps_cap` 的随手延迟，而挂件本身有淡入，看不出来。
12. **保留三条"跑不跑"的机制**，它们不改变帧率：轻晃关闭且链收敛后停帧（`charmRopeSettled`）、`document.hidden` 时暂停、窗口失焦 / 指针离开文档时冻结指针端（绝不瞬移）。轻晃开启时循环常驻、不按能量阈值停帧（那会把链冻在一个歪姿势上）；把幅度或频率调为 0、或系统要求减少动态时才完全停帧。轻晃默认仍开（0.8Hz / 幅度 20），省电档交给运营：把 `idle_sway_amp` 设 0 回零帧，或下调 `fps_cap`。
13. **指针端 = 指针热点 + `(pointer_offset_x, pointer_offset_y)`**，默认 `(3, 3)`，每轴合法区间 `-24–24`，单位 CSS px（与 `clientX/clientY` 同一视口坐标系，不随 DPI 缩放），允许负值，`0` 是普通合法值（指针端正好压在热点上、等于关掉偏移）。偏移只在物理头节点这一处换算，渲染不得再单独偏一次，否则弧垂与张力都会算在错的原点上。
14. **偏移钳位必须用 `normalizeSignedRange`**（夹到 `[min, max]`、NaN/±Inf 回落默认、`0` 与负数都是普通值）。`normalizePositiveRange` 把 `<= 0` 一律回落默认，`normalizeNonNegativeRange` 把负值夹回负数下界，两者都会静默吃掉负偏移，不得顶替。整数口径统一为"先夹边界、再取整"，半值远离 0（`-2.5 → -3`）；读者站 / 后台写成 `Math.sign(v) * Math.round(Math.abs(v))` 与 Go 的 `math.Round` 对齐。
15. **收尾描边用填充的带形多边形，不用一根等宽 `stroke`**。单根 SVG stroke 无法改变宽度；带形多边形沿中心线法向两侧偏移，宽度从起始线性收到固定 0.6 倍——收尾比例钉成常数而不进配置。代价是 `d` 比折线长，但每帧 DOM 写入仍然只有两个属性（路径的 `d` 与站标的一个 `transform`），不碰布局属性。

## 理由与已否决方案

- **把挂件塞进 `site` / 其余业务段**：装饰不属于站点身份或业务信息，混进去后"一次性关掉全部装饰"没有落点，且每加一种装饰都会污染业务段。已否决。
- **继续硬编码参数**：调参要发版、不能临时关闭，与本仓库"运行时配置 + 后台可改、改完下次请求即生效"的既有做法背离。已否决。
- **越界直接返回 400**：管理员只改了一半时无从判断哪项错了；钳位保证"库里永远存不下发散值"才是真正的安全边界。已否决。
- **纯解析悬链线**：它零抖动、零调参，也能给出弧垂，但站标会"死跟"指针，失去被绳子拖着的惯性感。已否决；保留 Verlet 链，用松弛换弧垂。
- **"调大重力来获得下垂"**：物理上错误——静止姿态就是竖直直线。这个直觉一旦写进参数说明，会误导所有后来人。已否决。
- **每帧一步的物理 / 物理跟着帧率降频**：帧率相关，120Hz 屏上快一倍；物理是固定步长按真实时间推进的，跟着降频省不到什么，却让"帧率"重新耦合到绳子手感上。已否决。
- **canvas / WebGL**：颜色与主题要手读计算样式、测试环境没有 canvas 实现。已否决。
- **把层级降到弹窗之下 / 做捏捏避让**：降层会失去"玻璃上的装饰"这一层意思；避让会让装饰显出智能与意图，违背"它不是功能入口"。已否决。
- **用单根 stroke 画收尾**：SVG stroke 不能沿路径变宽，伪宽度的 dash 技巧只会把收尾做成一串补丁。已否决。
- **自适应降频的任何变体（怠速车道、把 20fps 改成 30fps、用 `setTimeout` 做降频）**：闸门看的是输入（指针懒不懒），被采样的却是输出（绳子还在甩），病灶是两者错位；`setTimeout` 还不对齐 vsync、间隔抖动。已否决；只留一条 rAF 调度 + 时间门控。
- **"每帧算、只在到点发布"**：中间帧推进出来的状态永远不会被画出来，等于算了不画。已否决；用"早退不算"。
- **门控不带容差 / `pointermove` 里保留即时发布 / 首帧、`pointerover`、`focus` 开例外**：前者会让 60 档在 60Hz 屏上掉成 30 档；后两者给事件率最高的拖动路径开了后门，"帧率上限"名不副实。已否决。
- **放宽物理截断以支持更低的帧率**：那要重验"切后台不甩爆"，为几个不会有人选的档位不值得。已否决。
- **用 `0` 表示"不限" / 按帧计数跳帧**：后台表单纯元数据驱动，`min = 30` 时运营根本输入不了 0；按帧跳要先估刷新率，估错就整体偏一档。已否决。
- **指针端每步钉死在指针位置**：会把一帧里几十像素的指针位移当成瞬时冲量灌进链里，距离约束随即把每节拉到静止长度，快速甩动时"挂绳整条瞬直"。已否决；改为带阻尼跟随，把同一段位移摊到多个固定子步上，慢拖几乎贴手、快甩才落后。
- **偏移做成单标量（固定 45°）或"距离 + 方向" / 偏移只挪渲染起点**：前者将来只想横向偏就得改代码；后者参数膨胀；只挪渲染会让起点与绳子脱节。已否决。
- **只靠各端单测或读源码文本比范围 / 保留四源范围门禁与锚点清单**：各端内部自洽会掩盖手写事实漂移，源码解析又会跟着实现形状脆断。已否决；改由同一参数表生成四端消费物，用生成式校验与契约、后端 HTTP 接口的运行时取值验行为。
- **给装饰引入端到端帧率实测**：需要额外的浏览器依赖，超出本仓库现有测试分层；门控与偏移由注入假时钟 + 假动画帧的单测锁定即可。已否决。

## 代价 / 不做什么

- **`z-index: 9999` 的后果是它确实会画在登录弹窗与 toast 之上**。这是有意接受的；若将来觉得碍事，改的是一个数 + 一条设计门禁断言，不是重构。
- **参数表同时生成契约字段事实与两端常量**；共享归一化夹具（122 条用例）与后端 / 读者站 / 后台三套消费测试继续钉住夹边界、取整顺序与负半值远离 0。
- **钳位不报错意味着"错的输入会被静默纠正"**：管理员可能直到保存后看到被夹回的值才发现填错。接受这个代价，换取"不会因为一半没填好就整份被拒"。
- **恒定上限意味着"只是读文章"时轻晃开启也按上限重绘**。这是显式接受的代价；补偿是运营可把 `idle_sway_amp` 设 0（收敛后回零帧）或下调 `fps_cap`。
- **30 以下不开放**：物理截断不动，运营若填 29 会被钳到 30 而不是"生效"。高刷屏上后台填的数字是上限不是保证，后台文案必须说清，否则会被读成"配置没生效"。
- **偏移到 ±24 的极端值时，绳头与光标热点明显脱节**，视觉上像"断线"。这是有意的上界，不再放大范围。
- **阻尼上限为 1**：每个固定步长后速度完全不衰减，摆动会靠重力继续来回、可能持续振荡（大于 1 才是发散值、仍钳回 1）；后台文案必须写明这一点。
- **不做什么**：不做触屏与移动端适配、不做读者个人开关、不做实时预览、不引入第三方动画/物理库、不做第二种装饰、不做 canvas/WebGL 渲染路径、不把挂件做成第二个功能入口（捏捏仍是读者站 AI 的唯一入口）；不做"跟随显示器刷新率"这类布尔参数、不做按可见性/交互状态的第二套帧率、不引入配置实时推送（读者站仍是刷新生效）、不把 `fps_cap` 做成按主题或按页面、不为带阻尼跟随新增 `deco.charm` 参数（时间常数与滞后上界都是内部常量）、不改距离约束刚度口径（高速甩动时弦长仍可能被拉到绳长以上，是有意保留的弹性）。

## 当前生效约束

1. 装饰配置只放在 `deco` 段，挂件参数只在 `deco.charm` 子块；不得塞进 `site`/`ai`/其余业务段。新增装饰只加 `deco` 子块，不再开新段；`deco` 段本身由配置段注册表登记，注册表里不得另抄参数。
2. 写入路径的归一化**永不报错**：越界钳边界、缺省补默认、NaN/非法回落默认；不得把它改成返回 400。读者站与后台各自再做一次同样的钳位；后台每个数值参数是「滑块 + 数字框」双输入、读写同一字段，越界/非法输入按同一钳位规则夹回并给行内可见提示。
3. `deco.charm` 的 12 个数值参数与布尔总开关 `enable` 由后台可调；数值参数事实只由 `config/deco-charm-parameters.json` 定义，不得在任一消费端手抄 min/max/default。
4. 12 个数值参数必须来自同一张参数表，并由 `scripts/generate-deco-parameters.mjs` 生成后端、默认种子、读者站、后台四端消费物，min/max/default 注入契约 `conf.Charm`（开放接口契约）；`node scripts/generate-deco-parameters.mjs --check` 必须验「重新生成后四端无差异 + 运行时取值正确（`GET /site/deco` / `PUT /site/deco`）」，任何手写生成物或契约字段漂移都必须变红。旧四源逐项比范围门禁与锚点清单退场。
5. 共享归一化夹具（122 条用例）与后端 / 读者站 / 后台三套消费测试继续生效；整数参数先夹边界再取整、半值按远离 0 取整（`-2.5 → -3`），读者站 / 后台写成 `Math.sign(v) * Math.round(Math.abs(v))` 与 Go `math.Round` 对齐。`backend/docs/swagger.json` 与两端各自生成的 `app/types/openapi.d.ts` 必须同批重生成，禁止手改、禁止跨应用共享；不得为参数新增专用 swagger 路由或自由键值 DTO。
6. 弧垂只能来自"绳长大于指针端到站标的实际距离"；不得把 `gravity` 当作下垂旋钮，参数说明也不得暗示这一点。
7. 物理固定步长为 1/120s：单帧最多 8 个子步、累加器上限 0.05s；不得退回"每帧一步"，也不得为支持更低的 `fps_cap` 放宽这三个常量。
8. 指针端由带阻尼的一阶低通跟随驱动（时间常数由 `stiffness` 换算、滞后有硬上界、静止时精确吸附），`advanceCharmRope` 在固定子步之间对锚点线性插值；不得恢复"每步钉死在指针位置"。物理跟随实现在 `frontend/app/utils/charmPointerFollow.ts`，时间常数与滞后上界是内部常量、不得新增 `deco.charm` 参数；验收由 `test/utils/decoCharmPointerFollow.test.ts` 与 `test/composables/useCursorCharmPointerFollow.test.ts` 锁定平滑跟随、滞后有界不脱节、16.7ms 与 33.3ms 帧间隔末态一致。
9. 挂件只有一条调度：`requestAnimationFrame`。禁止引入 `setTimeout` / `setInterval` 驱动的帧，禁止 20fps 怠速车道、"指针移动期间走满速"或任何按指针是否活跃等运行时状态改变发布频率的闸门。
10. 帧率上限只来自 `deco.charm.fps_cap`（默认 60、区间 30–160、整数、单位 fps，语义是最高渲染帧率，高于显示器刷新率等价于每帧）；不得在前端写死帧率、不得新增第二个帧率参数、不得用 `0` 表示不限。
11. 门控判定是 `now - lastPublishAt >= 1000 / fps_cap - 4`；那个 4ms 容差不许删（删了 60 档在 60Hz 屏上会掉成 30 档）。未到点直接 `return`，不推进物理、不发布；`lastPublishAt` 初值视为已到点（是初值不是例外）；到点那帧按距上一次发布的真实流逝时间推进物理。
12. 唯一的发布口是门控帧：`pointermove` / `pointerover` / `focus` / 首帧都不得绕过，事件里只允许写 `pointer` 坐标与 `visible`；链在第一帧按当时的指针位置由 `ensureRope` 建立，不得在事件里建链。
13. 停帧（轻晃关闭且链收敛）、`document.hidden` 暂停、失焦 / 指针离开文档时冻结指针端（绝不瞬移）三条保留；删除任何一条都要另开 ADR。轻晃开启时循环常驻、不按能量阈值停帧；幅度或频率为 0、或系统开启减少动态时完全停帧。
14. 指针端 = 指针热点 + `(pointer_offset_x, pointer_offset_y)`；物理头节点必须用这个换算，渲染不得再单独偏一次。偏移每轴 `-24..24`、默认 `3/3`、单位 CSS px（与 `clientX/clientY` 同视口坐标系、不随 DPI 缩放）；`0` 与负值都是合法普通值。
15. 偏移钳位必须用 `normalizeSignedRange`，不得用 `normalizePositiveRange` / `normalizeNonNegativeRange` 顶替（它们会静默吃掉负值）；`damping` 上限为 1（不衰减、可能持续振荡，后台文案必须写明），越界值仍钳回 1。
16. 挂件层必须是全屏 SVG，且同时具备 `pointer-events: none`、`aria-hidden="true"`、`z-index: 9999`；颜色只消费语义 token（`--color-ink-faint`），不得出现裸色值；不得改用 canvas / WebGL。绘制几何在 `frontend/app/utils/charmTether.ts`，每帧 DOM 写入只有路径 `d` 与站标 `transform` 两个属性。
17. 启用条件三条同时满足：`(hover: hover) and (pointer: fine)`、系统未开启减少动态、后台总开关为真。触屏与减少动态下永不渲染。
18. 描边收尾固定为起始宽度的 0.6 倍、不进配置；必须用填充带形多边形，不得退回等宽 `stroke`。
19. 挂件不得成为第二个功能入口、不得进入 Tab 序列、不得避让捏捏；指针端离开窗口时只冻结、绝不瞬移。术语统一：链的第 0 个节点称「指针端节点」，不用「绳头」。
20. 不做触屏与移动端适配、不做读者个人开关、不做实时预览、不引入第三方动画/物理库、不做第二种装饰、不做 canvas/WebGL 渲染路径、不做"跟随显示器刷新率"参数、不做第二套帧率、不做配置实时推送、不把 `fps_cap` 做成按主题或按页面。

## 相关代码与文档

- 参数表与生成校验：[config/deco-charm-parameters.json](../../config/deco-charm-parameters.json)（唯一参数表）· [scripts/generate-deco-parameters.mjs](../../scripts/generate-deco-parameters.mjs) · [scripts/decorate-deco-contract.mjs](../../scripts/decorate-deco-contract.mjs) · [scripts/verify-gates.sh](../../scripts/verify-gates.sh) 第 14 道
- 配置与校验：[backend/conf/conf_deco.go](../../backend/conf/conf_deco.go)（后端消费物与四条钳位规则）· [backend/config/site_default_settings.yaml](../../backend/config/site_default_settings.yaml)（默认种子）· [backend/docs/swagger.json](../../backend/docs/swagger.json) · [frontend/app/types/openapi.d.ts](../../frontend/app/types/openapi.d.ts) · [frontend_admin/app/types/openapi.d.ts](../../frontend_admin/app/types/openapi.d.ts) · [配置分层 §五](../architecture/configuration.md)
- 读者站：[frontend/app/utils/decoCharm.ts](../../frontend/app/utils/decoCharm.ts)（生成常量与 clamp/normalize）· [frontend/app/utils/charmTether.ts](../../frontend/app/utils/charmTether.ts)（带形路径几何）· [frontend/app/utils/charmPointerFollow.ts](../../frontend/app/utils/charmPointerFollow.ts)（指针端跟随）· [frontend/app/composables/useCursorCharm.ts](../../frontend/app/composables/useCursorCharm.ts)（门控与唯一发布口）· [frontend/app/components/deco/CursorCharm.vue](../../frontend/app/components/deco/CursorCharm.vue)
- 后台：[frontend_admin/app/utils/decoParams.ts](../../frontend_admin/app/utils/decoParams.ts)（生成元数据，表单纯元数据驱动）· [frontend_admin/app/types/api.ts](../../frontend_admin/app/types/api.ts)
- 门禁与夹具：[backend/conf/testdata/deco-charm-normalization.json](../../backend/conf/testdata/deco-charm-normalization.json)（122 条用例）· [frontend/test/utils/decoCharmPointerFollow.test.ts](../../frontend/test/utils/decoCharmPointerFollow.test.ts) · [frontend/test/composables/useCursorCharmPointerFollow.test.ts](../../frontend/test/composables/useCursorCharmPointerFollow.test.ts)
- 上游：[ADR-0011](./0011-neutral-glass-visual-system.md)（视觉系统与动效预算）· [ADR-0010](./0010-jelly-replaces-traditional-ai-entry.md)（捏捏是读者站 AI 的唯一入口）· [ADR-0003](./0003-generated-contract-single-source.md)（契约以代码生成为单一来源）· [spec issue #35](https://github.com/Jenlybein/NewblogX/issues/35)
- 词汇：[GLOSSARY · 光标挂件 / 挂绳 / 指针端](../../GLOSSARY.md)
