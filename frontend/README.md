# BlogX Web

`apps/web` 是 BlogX 的公开站点与用户侧前端，基于 `Nuxt 4 + Vue 3 + Pinia + Naive UI + Tailwind CSS`。

## 当前结论

这个项目现在正式按下面的方式运行：

- 开发：`Nuxt dev + Go API`
- 测试/生产：`Nginx + Nuxt Node SSR + Go API`

浏览器公开入口固定为同源路径：

- HTTP API：`/api/*`
- WebSocket：`/api/chat/ws`
- 上传与静态资源：`/uploads/*`

> 历史上还有 `/_origin/*`（资源源站）与 `/_backend/*`（旧 API 别名）两个路径，
> 它们都只是把请求转发到后端根目录，与 `/uploads`、`/api` 完全重复，已随 S5.5 删除。
> 后端只把 `uploads` 目录挂成静态（`r.Static("/uploads", "./uploads")`），没有根路径静态。

测试/生产的 SSR 部署里：

- `/` -> Nuxt SSR 容器
- `/api/*` -> Go API
- `/api/chat/ws` -> Go WebSocket
- `/uploads/*` -> Go

## 环境文件

仓库内置 4 套 web profile：

- [common.env](/E:/project/blogx_monorepo/apps/web/env/common.env)
- [local-local.env](/E:/project/blogx_monorepo/apps/web/env/local-local.env)
- [local-test.env](/E:/project/blogx_monorepo/apps/web/env/local-test.env)
- [test.env](/E:/project/blogx_monorepo/apps/web/env/test.env)
- [production.env](/E:/project/blogx_monorepo/apps/web/env/production.env)

含义如下：

1. `local-local`
   页面：`http://localhost:3000`
   API：`http://localhost:3000/api/*`
   实际上游：`http://127.0.0.1:8080`
2. `local-test`
   页面：`http://localhost:3000`
   API：`http://localhost:3000/api/*`
   实际上游：`https://blog.gentlybeing.cn`
3. `test`
   页面：`https://blog.gentlybeing.cn`
   API：`https://blog.gentlybeing.cn/api/*`
   默认构建口径：测试站
4. `production`
   页面：`https://blogx.gentlybeing.cn`
   API：`https://blogx.gentlybeing.cn/api/*`
   默认构建口径：生产站

前端环境切换只认 `apps/web/env/*.env` 和根目录 `.envrc`。

## 如何切换环境

仓库根目录：

```bash
pnpm dev:web:local-local
pnpm dev:web:local-test
pnpm build:web:test
pnpm build:web:production
pnpm start:web
```

`apps/web` 目录：

```bash
pnpm dev:local-local
pnpm dev:local-test
pnpm build:test
pnpm build:production
pnpm start
```

这些脚本本质上只是设置 `BLOGX_WEB_ENV_PROFILE`，再运行 Nuxt 命令。

## 开发环境怎么工作

开发时浏览器只访问 `http://localhost:3000`。

- 页面由 Nuxt dev server 提供
- `/api/*` 由 Nuxt 服务端代理到 `BLOGX_WEB_API_UPSTREAM`
- `/api/chat/ws` 由 Nuxt `devProxy` 转发
- `/uploads/*` 也保持同源

所以“本地前端 + 测试后端”不需要单独处理 CORS，只需要切到 `local-test`。

## 测试与生产部署

### 前端容器

Nuxt SSR 容器 Dockerfile 在：

- [deploy/docker/web/Dockerfile](/E:/project/blogx_monorepo/deploy/docker/web/Dockerfile)

镜像构建阶段执行 `nuxt build`，运行阶段启动：

```bash
node .output/server/index.mjs
```

容器镜像不再通过 Docker `build.args` 烘焙环境配置；compose 会在运行阶段把现有 `BLOGX_*`
映射为 Nuxt 标准的 `NUXT_*` 运行时变量：

- `BLOGX_WEB_API_UPSTREAM` -> `NUXT_API_UPSTREAM`
- `BLOGX_WEB_SITE_URL` -> `NUXT_PUBLIC_SITE_URL`
- `BLOGX_WEB_API_BASE` -> `NUXT_PUBLIC_API_BASE`
- `BLOGX_WEB_WS_PATH` -> `NUXT_PUBLIC_WS_PATH`
- `BLOGX_WEB_ASSET_PROXY_BASE` -> `NUXT_PUBLIC_ASSET_PROXY_BASE`

因此同一个 `blogx-web:1` 镜像可以在不同运行环境下复用；改 upstream/siteUrl 后重启容器即可生效，
不需要为了这些运行时配置重新 build。

### Nginx 配置（单模板 + envsubst）

Nginx 只保留一份模板，启动时由 `envsubst` 按部署形态渲染：

- 模板：[nginx.conf.tmpl](../../deploy/docker/nginx/nginx.conf.tmpl)
- 渲染脚本：[render-config.sh](../../deploy/docker/nginx/render-config.sh)

容器启动时执行 `render-config.sh`，用 `envsubst` 把模板渲染成 `/etc/nginx/nginx.conf` 再启动 nginx。
只需要一个开关 `BLOGX_NGINX_ENV` 选择形态，其余变量（server_name、TLS、upstream、是否启用 web/admin）由脚本推导：

- `BLOGX_NGINX_ENV=local`（默认）-> `localhost`，不启用 TLS
- `BLOGX_NGINX_ENV=test` -> `blog.gentlybeing.cn`，启用 TLS
- `BLOGX_NGINX_ENV=production` -> `blogx.gentlybeing.cn`，启用 TLS
- `BLOGX_NGINX_ENV=api-only` -> `blog.gentlybeing.cn`，启用 TLS，且 `/admin` 与 `/` 返回 404

显式传入同名环境变量（如 `BLOGX_NGINX_SERVER_NAME`、`BLOGX_NGINX_WEB_UPSTREAM`）可临时覆盖脚本默认值。

`api-only` 用于“服务器跑后端 Docker 容器，本地跑 Nuxt dev server”的联调模式；它只代理 `/api/*`、`/api/chat/ws`、`/uploads/*`，不依赖 `blogx_web`。

### 本地 compose

[deploy/compose/local/docker-compose.yml](/E:/project/blogx_monorepo/deploy/compose/local/docker-compose.yml) 现在是完整的 SSR 链路：

- `blogx_web`：Nuxt Node SSR
- `blogx_server`：Go API
- `nginx`：反代入口

启动后链路是：

```text
Browser -> nginx -> blogx_web / blogx_server
```

## 类型检查

```bash
pnpm --filter web exec nuxi typecheck
```
