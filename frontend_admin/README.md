# BlogX Admin

BlogX 后台管理端，默认挂载在 `/admin/`。

## 开发

```bash
pnpm dev
pnpm dev:local-local
pnpm dev:local-test
pnpm build:test
pnpm build:production
```

这些脚本会设置 `BLOGX_ADMIN_ENV_PROFILE`，再运行 Nuxt 命令。

## 容器运行时配置

Admin 容器镜像不再通过 Docker `build.args` 烘焙环境配置；compose 会在运行阶段把现有
`BLOGX_ADMIN_*` 映射为 Nuxt 标准的 `NUXT_*` 运行时变量：

- `BLOGX_ADMIN_API_UPSTREAM` -> `NUXT_API_UPSTREAM`
- `BLOGX_ADMIN_SITE_URL` -> `NUXT_PUBLIC_SITE_URL`
- `BLOGX_ADMIN_API_BASE` -> `NUXT_PUBLIC_API_BASE`
- `BLOGX_ADMIN_WEB_SITE_URL` -> `NUXT_PUBLIC_WEB_SITE_URL`

同一个 `blogx-admin:1` 镜像可以在不同运行环境下复用。`/admin/` 是后台资源路径的构建约束，
不要在同一个已构建镜像里随意切成 `/`，否则静态资源路径可能不一致。
