# ADR-0037：关注关系收成三视图与单向移除粉丝

- 状态：已接受（本批随 PR #327 合并后转「已交付」）
- 日期：2026-11-10
- 来源：用户 2026-11-10 的五个决策（统一读端点 / 单向移除 / 带计数 / 契约一把改完 / 与当前 PR 一起做到理想形）· 落地票 [#327](https://github.com/Jenlybein/NewblogX/issues/327) · 关联 [ADR-0003](./0003-generated-contract-single-source.md)（契约单一来源）· [ADR-0007](./0007-id-string-type-as-truth.md)（ID 即 string）· 术语见 [GLOSSARY · 关系视图 / 移除粉丝](../../GLOSSARY.md)

## 背景与问题

关注与粉丝本来是**两个读端点、两种条目形状**：

- `GET /api/follow` → `FollowingItem`（`followed_user_id` / `followed_nickname` / …），另有 `followed_user_id` 过滤参数；
- `GET /api/fans` → `FansItem`（`fans_user_id` / `fans_nickname` / …），另有 `fans_user_id` 过滤参数。

这两个过滤参数**没有任何消费方**（两个前端只用 `user_id` + 分页），却让契约多出两种形状。落到前端，同一种"一个人的关系卡片"要写两套归一（`normalizeFollow` / `normalizeFan`），谁改字段谁漏一边。

真正的痛点在三处：

1. **互关没有端点。** 互关 = 我关注的人 ∩ 关注我的人。它没有独立数据，只能靠"取一页关注列表、在客户端筛 `relation === 4`"来凑。这样凑出来的页比服务端一页少，`has_more` 与每页条数都不可信；为凑够一页还得向后继续扫（本批之前临时上限 6 页），关注几千人时这个视图基本不可用。
2. **没有计数。** 顶栏想做"关注 128 / 粉丝 96 / 互关 24"，只能靠翻页数条数，翻到第几页数字就变一次。
3. **没有"移除粉丝"。** 后端只有"取消我自己的关注"（`DELETE /api/follow/{id}`），没有"让关注我的人消失"的能力。用户在截图的菜单里点了「移除粉丝」，实际无处可点。

## 决策

1. **读只有一个入口：`GET /api/relations?type=follow|fans|mutual`。** `type` 必填（三个视图语义不同，猜默认值只会让调用方静默拿到错数据），只接受这三个取值。省略 `user_id` 看自己，给了就看他人的列表（受对方可见性设置约束）。
2. **条目只有一种形状：`RelationItem`。** 字段 `user_id / nickname / avatar / abstract / follow_time / relation`——不带方向前缀，因为**方向属于视图，不属于条目**。`FollowingItem` / `FansItem` 与其包装形状随之删除。"每行出现哪一端"收在领域层一处：`Follow.Counterpart(kind)`（fans 取粉丝端，follow / mutual 取被关注端）。
3. **互关在 SQL 里先筛后分页。** `fans_user_id = 我 AND followed_user_id IN (SELECT fans_user_id WHERE followed_user_id = 我)`，再 `LIMIT/OFFSET`。反过来"先分页后过滤"会让每页条数随关系分布抖动，是这一票要消掉的东西。
4. **计数端点是 `GET /api/relations/counts`**，一次返回三个数（`follow` / `fans` / `mutual`），形状参照既有未读汇总（`GET /api/sitemsg/user`）。计数与列表**同一套可见性判据**：列表不可见时计数也失败，否则等于把私密关系的规模透给访客。
5. **`DELETE /api/relations/fans/{id}` 是单向移除。** 只删"他关注我"这一条：不通知对方、不阻止他再次关注、不影响我关注的人。持久化上它与取关是同一个操作（`Unfollow(followed, fans)` 的参数顺序不同），语义差别只在用例层。统计按同一方向回滚（他的关注数 -1、我的粉丝数 -1）。新增错误码 `follow.fan_not_found`（"该用户不是你的粉丝"），登记为 `ProtocolValidation`——它是调用方能看懂并放弃的失败，不是服务异常。
6. **写仍然在 `/follow/{id}`。** 路径按"读 / 写"分开：`/follow` 是"我对某人的动作"（关注 / 取关），`/relations` 是"关系的三个视图"，`/relations/fans/{id}` 是"我对关注我的人的动作"。不把写并进读端点，是因为两者的授权与审计含义不同。
7. **契约按实测重新登记：`138/179/338 → 139/180/337`。** paths +1、operations +1、schemas **-1**。这 1 个 schema 的下降是**有意**的净额：删 6（两种旧形状 + 各自的 `HasMoreListData` 与 `Envelope`）增 5（一种新形状 + 一份包装）。按仓库约定（`scripts/contract-structures.json` 的 comment）下降必须在 comment / ADR / 提交说明三处写明理由，本文件即第二处。
8. **公开主页保持只读。** 「移除粉丝」只出现在个人中心；`/users/[id]` 的关注 / 粉丝分栏复用同一个读端点，但没有任何写入口。

## 理由与已否决方案

- **否决"保留两个端点，在 `/api/follow` 上加 `mutual` 参数"**：改动面确实更小，但"两个端点、两种形状"仍在，前端两套归一也仍在；粉丝侧要筛互关还得再加一个参数，形状分叉会一直留在契约里。这一票的目标就是消掉分叉，不是把它减半。
- **否决"继续在客户端筛互关"**：分页失真不是性能问题而是正确性问题——筛出来的一页不等于服务端的一页，`has_more` 也不可信，于是只能靠"多扫几页"补，写死上限就永远有"互关比实际少"的边界。
- **否决"让移除粉丝走物理删除"**：关系行本来就走 GORM 软删（`BaseRow`），物理删除会让"恢复"这条既有能力断掉，也与全仓口径不一致。删（软删）一行已经足够表达"这条关系不存在了"。
- **否决"给移除粉丝单独建表"**：关系本身就是那张表的一行，删行即语义；另建表要么重复存关系，要么只存"曾被我移除"的黑名单——后者是"不再接收此人的关注"的语义，属于**拉黑**，不是本票要的东西。
- **否决"这次就做拉黑"**：拉黑要新增关系态、双向可见性计算与"禁止再次关注"的校验，还要回答"被拉黑的人看到什么"。它是一个独立决策，塞进这一票会让"移除粉丝"这个明确的小动作被大语义拖住。单向移除先落地，拉黑另开。
- **否决"计数读冗余列"**：`user_stat` 里有 `fans_count` / `follow_count`，但**没有互关计数**，零 DDL 下不能加列；而且这些列是派生值（[ADR-0025](./0025-count-ledger-and-reconciliation.md)），拿它当权威会与关系行不一致。三条 COUNT 现算，代价可接受。

## 代价 / 不做什么

- **每次打开关系页多三条 COUNT**（关注 / 粉丝 / 互关）。计数与列表是两个请求，因此页面会先出列表、后出徽标；徽标读失败只是少个数字，不影响列表。
- **互关用相关子查询**，关系量极大时比两条索引扫描贵。个人站的关系量级下这不是瓶颈；真要优化应先在 `user_follow_models` 上确认索引覆盖，而不是把过滤搬回应用层。
- **本票不保留旧端点的过渡期。** `GET /api/follow` 与 `GET /api/fans` 直接删除（调用方只有本仓两个前端，同批切换完）。外部若有脚本在读这两个端点会得到 404——这是"一把改完"的代价，用户明确选择了它。
- **单向移除对方无感**：他被移除后，界面上的关注数会静默 -1，可能困惑。这是该语义的固有代价，`_Avoid_` 之外没有补偿机制（不通知、不留痕）。
- **不做**：拉黑（禁止再次关注）、移除粉丝的撤销、关系列表的搜索 / 排序选择、批量移除。都要么是独立决策，要么需要新列 / 新表。

## 当前生效约束

1. 关系读**只能**走 `GET /relations`；不得再新增 `/follow` 或 `/fans` 形式的读端点。
2. `type` 必填且只接受 `follow|fans|mutual`；不认识的取值一律拒绝，不许回落成某个默认视图。
3. 互关必须在**持久化层**先筛后分页；禁止把过滤搬到用例层或客户端再做一次。
4. 关系条目只允许一种对外形状（`RelationItem`）；"取哪一端"只能由 `Follow.Counterpart` 决定，不许在适配器或前端各写一份。
5. 移除粉丝只能删除"他关注我"这一条：不得影响我关注的人，不得物理删除，统计必须同方向回滚。
6. 计数与列表共用同一套可见性判据；列表不可见时计数不得返回数字。
7. 公开主页（`/users/[id]`）只读：任何关系写入口只允许出现在个人中心。
8. 契约条目数若下降，必须在 `scripts/contract-structures.json` 的 comment、本 ADR、提交说明三处写明预期理由，并同时抬高 / 重新登记基线。

## 相关代码与文档

- 领域：`backend/internal/social/domain/social.go:45`（`RelationKind`）· `:60`（`ParseRelationKind`）· `:76`（`RelationItem`）· `:89`（`RelationCounts`）· `:101`（`Follow.Counterpart`）· `backend/internal/social/domain/port.go:11-24`（端口方法）
- 持久化：`backend/internal/social/adapter/mysql/follow.go:85`（`ListRelations`，三视图共用一条路径）· `:131`（`CountRelations`）· `:159`（`count`）
- 用例：`backend/internal/social/app/service.go:102`（`ListRelations`）· `:121`（`CountRelations`）· `:143`（`RemoveFan`）· `:169`（`ensureVisible`）· `:195`（`toRelationItems`）
- 接口：`backend/internal/social/adapter/http/relations.go:50` / `:79` / `:102` · 路由 `backend/internal/platform/http/router/follow_router.go:33-36` · 错误码 `backend/internal/apperr/apperr.go`（`follow.fan_not_found`）与 `backend/internal/apperr/protocol.go`（`ProtocolValidation`）
- 前端：`frontend/app/pages/studio/relations.vue:81`（取数）· `:124`（切视图）· `:155` / `:165`（移除粉丝与二次确认）· `frontend/app/components/social/RelationCard.vue`（卡片与「更多」菜单）· `frontend/app/services/follow.ts`（三个端点）· `frontend/app/components/profile/ProfileRelationList.vue`（公开主页只读复用）
- 契约：`backend/docs/swagger.json` · `frontend/app/types/openapi.d.ts` · `frontend_admin/app/types/openapi.d.ts` · `scripts/contract-structures.json`（基线 139/180/337 与 ticket 327 批）
- 用例：`backend/internal/social/app/service_test.go`（用例层选择）· `backend/internal/social/domain/social_test.go`（枚举与 `Counterpart`）· `backend/internal/social/adapter/mysql/follow_integration_test.go`（真 MySQL：三视图 / 计数 / 单向移除）· `frontend/test/pages/studioRelationsPage.test.ts` · `frontend/test/components/RelationCard.test.ts`
- 术语：[GLOSSARY · 关系视图 / 移除粉丝](../../GLOSSARY.md)
