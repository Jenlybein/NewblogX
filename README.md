# blogx

解耦的单体仓库：Go 后端 + 两个独立的 Nuxt 前端 + 基础设施。**不是 monorepo**——没有 workspace、没有 turbo、没有共享代码包。

## 目录

| 目录 | 说明 | 技术栈 |
| --- | --- | --- |
| `backend/` | 后端 API | Go 1.25 · Gin · GORM |
| `frontend/` | 读者站 | Nuxt 4 |
| `frontend_admin/` | 管理后台 | Nuxt 4 |
| `deploy/` | Docker / compose / nginx（反向代理唯一 upstream 定义处） | — |
| `docs/architecture/` | 架构评审与重构设计 | — |

## 开发

三部分各自独立安装与启动：

```bash
# 后端
cd backend && go run ./cmd/server

# 读者站
cd frontend && pnpm install && pnpm dev

# 管理后台
cd frontend_admin && pnpm install && pnpm dev
```

## 前后端契约

后端用 `swag` 从代码自动生成 `backend/openapi.json`；两个前端各自用 `openapi-typescript` 生成类型到 `src/api/generated/api.gen.ts`。

契约的单一来源是后端代码，前端类型全部派生得到，**不手写、不跨 app 共享代码**。
