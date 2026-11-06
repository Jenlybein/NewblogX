# 2026-11-05 消息中心重构 · 部署记录

- 规格：[#270](https://github.com/Jenlybein/NewblogX/issues/270)（已关闭）
- PR：[#271](https://github.com/Jenlybein/NewblogX/pull/271)（一个 PR 整批）
- CI：run `41892933631`，13/13 全绿（archcheck 含测试文件、contract up-to-date、两端契约类型、integration 均通过）
- 合并提交：`1bc217c`（Merge pull request #271）
- 部署前 HEAD：`0ddbe73` → 部署后 `1bc217c`

## 串行部署（服务器 `腾讯云blog` / `/opt/blogx`）

```bash
git merge --ff-only origin/main
cd deploy/compose/production && set -a && . ./.env && set +a
docker compose up -d --build blogx_server     # 后端新契约
docker compose up -d --build blogx_web        # 读者站重构
docker compose up -d --build blogx_admin      # 后台契约类型同步
docker compose restart nginx
```

**本批没有新表、没有列变更、没有 ALTER，未跑 `-db`。** 数据库表数仍是 40。

部署后四个容器状态：`blogx-server` / `blogx-web` / `blogx-admin` 均 healthy，`nginx` 正常重启。

## 产物验证

| 检查 | 结果 |
| --- | --- |
| `GET /api/chat/messages?...&limit=7`（未登录） | `请先登录`（中间件正常，契约已生效） |
| 读者站 bundle 含 `chatTimeline` / `next_cursor` | 3 处命中 |
| 读者站 bundle 残留「预备会话 / HTTP 收发与落库 / 按时间倒序排列。」 | **0 处** |
| 读者站 bundle 含 `message-center__content`（顶部导航条 + 单一内容卡） | 3 处命中 |
| 顶栏 bundle 含 `useHeaderMessagesPanel` / `site-action--equal` | 4 处命中 |
| `https://gentlybeing.cn/` | 200 |
| `https://gentlybeing.cn/admin/` | 200 |
| 本机 `/health` | 200 |

## 本地 dev 截图

见 [发布说明](./2026-11-05-message-center-redesign-release-notes.md#本地-dev-关键态截图)：亮色私信、暗色私信、亮色评论与回复三张。用本地 dev server + fixture 数据渲染（非线上截图），仅供观感确认。

## 待人工验收

按发布说明末尾的「两个浏览器（一个无痕）」清单逐项走：升序、7 条上滑、进会话即已读两处角标清零、日期分组、异常状态条、顶栏悬浮面板可交互、四个 tab 进入即已读。
