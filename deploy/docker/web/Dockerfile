# syntax=docker/dockerfile:1.7

# frontend 为自洽应用：构建只依赖自身 package.json / pnpm-lock.yaml，不再使用 workspace。
# 构建上下文为仓库根目录（与 api Dockerfile 保持一致）。
FROM node:20-alpine AS builder

WORKDIR /build/frontend

ARG NPM_REGISTRY=https://registry.npmmirror.com
ENV COREPACK_NPM_REGISTRY=${NPM_REGISTRY}
ENV npm_config_registry=${NPM_REGISTRY}
ENV PNPM_HOME=/pnpm
ENV PATH=${PNPM_HOME}:${PATH}

RUN corepack enable && corepack prepare pnpm@10.33.0 --activate

COPY frontend/package.json frontend/pnpm-lock.yaml ./

RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm install --frozen-lockfile --store-dir=/pnpm/store

COPY frontend ./

RUN pnpm exec nuxt build

FROM node:20-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production
ENV NITRO_HOST=0.0.0.0
ENV NITRO_PORT=3000

COPY --from=builder /build/frontend/.output ./.output

EXPOSE 3000

CMD ["node", ".output/server/index.mjs"]
