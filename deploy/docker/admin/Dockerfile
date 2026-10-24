# syntax=docker/dockerfile:1.7

# frontend_admin 为自洽应用：构建只依赖自身 package.json / pnpm-lock.yaml，不再使用 workspace。
# 构建上下文为仓库根目录（与 api Dockerfile 保持一致）。
FROM node:20-alpine AS builder

WORKDIR /build/frontend_admin

# 层序约束（勿调换）：
#   1) 先只 COPY 依赖清单并 pnpm install —— 依赖层，源码改动不会击穿它；
#   2) 再 COPY 全量源码并 build —— 源码层，只有它随源码变化。
# 若把 COPY ./（全量源码）挪到 install 之前，每次改一行代码都会重装全部依赖。
ARG NPM_REGISTRY=https://registry.npmmirror.com
ENV COREPACK_NPM_REGISTRY=${NPM_REGISTRY}
ENV npm_config_registry=${NPM_REGISTRY}
ENV PNPM_HOME=/pnpm
ENV PATH=${PNPM_HOME}:${PATH}

RUN corepack enable && corepack prepare pnpm@10.33.0 --activate

COPY frontend_admin/package.json frontend_admin/pnpm-lock.yaml ./

RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm install --frozen-lockfile --store-dir=/pnpm/store

COPY frontend_admin ./

RUN pnpm exec nuxt build

FROM node:20-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production
ENV NITRO_HOST=0.0.0.0
ENV NITRO_PORT=3001

COPY --from=builder /build/frontend_admin/.output ./.output

EXPOSE 3001

CMD ["node", ".output/server/index.mjs"]
