# ADR-0035：评论管理（删除 / 禁言 / 封禁）与读者站行内删除

- 状态：已接受
- 日期：2026-11-27
- 来源：垃圾广告评论处置（2026-11-27 线上：某账号在文章下连发三条广告，后台没有入口，只能删库）· 关联 [ADR-0032](./0032-content-filter.md)（评论一律发布、命中的处置是打星）· [ADR-0029](./0029-admin-nav-three-levels-and-site-split.md)（菜单归属：文章管理 → 评论管理）· [ADR-0034](./0034-image-display-styles-and-audit-gate.md)（同一条"处置入口 + 审计"的范式）
- 术语：[GLOSSARY · 评论管理](../../GLOSSARY.md)

## 背景与问题

读者站的评论是"一律发布"的（ADR-0032 决策 2：没有审核中状态、没有人工队列）。这套设计
换来的是"评论不会被卡住"，代价是**一旦有人刷广告，只能靠人处理**。而处理入口此前只有两个：

1. 作者在文章页删自己文章下的评论 —— 后端 `DELETE /api/comments/:id` 早就有，权限
   （`domain.CanDelete`）也早就有"管理员 / 评论作者 / 文章作者"三条分支，但**读者站没有按钮**；
2. 管理后台没有任何评论页面 —— 运营遇到刷屏账号，只能删库软删 + 手工同步
   `article_models.comment_count`。2026-11-27 就这样处理过一次。

要解决的不是"删一条评论"，而是三件不同的事：**找到它**（在 6 万条评论里定位）、
**删掉它**（这一次）、**处理这个人**（下一次不来）。

## 决策

### 一、管理端新增「文章管理 → 评论管理」（`/articles/comments`）

- **列表**：五维筛选 —— 内容关键词、用户（ID 或昵称）、文章 ID、时间区间、状态（全部 / 已发布 / 已删除）；
  最新在前，分页只回 `has_more`（不造假总数：运营只需要"还能不能往下翻"）。
- **删除**：单条与批量（1–100 条）。**逐条回报失败**，不整单回滚：一次选中"一级评论 + 它下面的回复"时，
  一级的级联已经把回复删掉，重复项按成功算（目标状态已达成，报失败会让运营以为要再点一次）。
- **禁言**：1 / 7 / 30 天或永久，理由必填（≤200 字，进处置记录）。只拦发布类动作。
- **封禁**：账号状态置封禁 + 吊销会话，理由可选。与「用户管理」的封禁是**同一个动作**。
- 三个写动作都写 `action_audit_logs`（`comment_delete_admin` / `comment_mute_user` / `comment_ban_user`）。

### 二、后端四个管理员端点（`/api/comments/admin/*`）

`GET /list`、`DELETE /`（批量）、`POST /mute`、`POST /ban`。都挂在 `AdminMiddleware` 下，
写接口挂 `CaptureLog(ReqBody)`。

分层照既有范式：`domain`（规则 + 端口）/ `app`（用例）/ `adapter/mysql`（`ListAdmin`）/
`adapter/http`（DTO + swag）/ `app/comment_wiring.go`（文章标题批量读 + 账号处置桥接）/
`platform/router`。

### 三、读者站文章页给行内删除

可见性 = 管理员 / 评论作者 / 文章作者（与后端 `CanDelete` 同一口径），二次确认文案写明
"删除一级评论会连同它下面的全部回复一起删除"。删除成功**不本地假删**，按点赞同一口径重取
对应楼层（根评论 → 根评论页；子评论 → 该层当前回复页）—— 评论计数由后端维护，前端算不出来的。

## 三个实现判断（代价都写在注释里）

1. **"已删除"的真实标记是 `deleted_at`，不是 `status`**。`SoftDeleteMany` 只写 `deleted_at`，
   `status` 仍是 2；只有启动迁移会把存量改动 `status=3`。列表若只看 `status`，"已删除"筛选
   永远为空。做法：`published` = `deleted_at IS NULL AND status=2`，对外仍报 `status=3 + deleted=true`。
   真 MySQL 集成测试钉住这条事实（不靠约定）。
2. **「永久禁言」不能换算成 3650 天**。user 的 `NormalizeMuteDays` 上限是 365，换算会被拒。
   新增 `MuteForever`（写 DATETIME 上界 9999-12-31），桥接层 `days==0 → MuteForever`。
3. **理由暂不发给用户**。user 的 `Mute` 没有 reason 参数、通知链路不在本期；理由只进审计。
   要发给用户需要另开一条通知，不能靠"审计里有"当作用户可见。

## 已否决

- **否决"评论也要人工审核"**：ADR-0032 决策 2 刚把评论审核退场（评论区卡住比误发一条更伤），
  这次的解法是"事后处置"，不是"事前审核"。
- **否决在管理端另写一条删除路径**：计数与级联（同根回复、点赞、`comment_count`）已经在
  `Service.Remove` 里收敛，另开一条必然漂移。管理端与作者侧走同一个用例。

## 代价 / 不做什么

- 不做评论的申诉与恢复（与 ADR-0032 一致：不提供误报恢复通道）。
- 不做按评论内容的全文检索（关键词是 WHERE LIKE 子串；数据量到百万级再换）。
- 不做"删除理由"发给被删评论的作者（见判断 3）。

## 当前生效约束

- 所有评论删除（作者侧、管理端、未来的任何入口）都必须走 `comment/app.Service.Remove`；
  禁止在 adapter 或别的模块里直接 `SoftDeleteMany`。
- 管理端列表的"已删除"判据必须同时看 `deleted_at` 与 `status`，不得只看其一。
- 封禁/禁言的能力只在合成根桥接（`app/comment_wiring.go`），`comment` 模块不 import `user`。

## 相关代码与文档

- `backend/internal/comment/{domain,app,adapter}`、`backend/app/comment_wiring.go`
- `backend/internal/user/app/mute_permanent.go`
- `frontend_admin/app/components/articles/CommentManager.vue`、`app/pages/articles/comments.vue`
- `frontend/app/composables/useArticleComments.ts`、`app/components/comment/CommentThread.vue`
- `backend/internal/apperr`（`comment.admin_*` 与 `article.content_blocked` 同一套登记口径）
