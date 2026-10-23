# 阶段 1：基于固定版本的 Go + Alpine 镜像创建构建环境
FROM golang:1.25.1-alpine3.22 AS builder

# 禁用 CGO，生成静态可执行文件
ENV CGO_ENABLED=0

# 配置国内镜像，加速依赖下载
ENV GOPROXY=https://goproxy.cn,direct

# 修改 alpine 软件源，加速 apk 安装
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories

# 先复制 go mod 文件，利用 Docker layer cache 加速依赖下载
WORKDIR /build/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# 再复制应用源码并编译服务入口
COPY backend ./
RUN go build -o /build/out/server ./cmd/server

# 阶段 2：基于固定版本的 Alpine 镜像创建运行环境
FROM alpine:3.22

# 设置运行时工作目录
WORKDIR /app

# 拷贝编译产物和应用默认资源
COPY --from=builder /build/out/server /app/server
COPY --from=builder /build/backend/config /app/config
COPY --from=builder /build/backend/resources /app/resources

# 安装运行时依赖：时区、CA 证书、mysql 客户端（包含 mysqldump）
RUN apk add --no-cache tzdata ca-certificates mysql-client && update-ca-certificates

# 预创建运行时目录，便于卷挂载和 river 位点持久化。
# 另外在 /usr/local/bin 放一个同名 mysqldump wrapper，这样容器内仍可使用配置值 "mysqldump"，
# 但最终会自动转发到系统真实二进制，并补上兼容参数。
RUN printf '%s\n' \
	'#!/bin/sh' \
	'set -eu' \
	'' \
	'for candidate in /usr/bin/mariadb-dump /usr/bin/mysqldump; do' \
	'	if [ -x "$candidate" ]; then' \
	'		exec "$candidate" --skip-ssl-verify-server-cert "$@"' \
	'	fi' \
	'done' \
	'' \
	'echo "未找到可用的 mysqldump/mariadb-dump 可执行文件" >&2' \
	'exit 127' \
	> /usr/local/bin/mariadb-dump-wrapper \
	&& cp /usr/local/bin/mariadb-dump-wrapper /usr/local/bin/mysqldump \
	&& chmod +x /usr/local/bin/mariadb-dump-wrapper \
	&& chmod +x /usr/local/bin/mysqldump \
	&& mkdir -p /app/logs /app/uploads /app/var

# 容器默认只执行服务进程；部署层可按需覆盖配置文件与角色。
CMD ["/app/server"]
