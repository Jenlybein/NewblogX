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
# cli 一起编译：ES 索引的建/删/全量同步只有 cli 能做（服务端启动不会建索引），
# 没它就会出现"索引由首次写入自动建出来、没有中文分词"的裸索引。
COPY backend ./
RUN go build -o /build/out/server ./cmd/server && go build -o /build/out/cli ./cmd/cli

# 阶段 2：基于固定版本的 Alpine 镜像创建运行环境
FROM alpine:3.22

# 设置运行时工作目录
WORKDIR /app

# 拷贝编译产物和应用默认资源
COPY --from=builder /build/out/server /app/server
COPY --from=builder /build/out/cli /app/cli
COPY --from=builder /build/backend/config /app/config
COPY --from=builder /build/backend/resources /app/resources

# 安装运行时依赖：时区、CA 证书、mysql 客户端（包含 mysqldump）
#
# 注意：Alpine 的 mysql-client 包其实就是 MariaDB 客户端，所以容器里的
# mysqldump 实际是 mariadb-dump。两者对 MySQL 服务端并不完全兼容（见下）。
RUN apk add --no-cache tzdata ca-certificates mysql-client && update-ca-certificates

# 预创建运行时目录，便于卷挂载和 river 位点持久化。
# 另外在 /usr/local/bin 放一个同名 mysqldump wrapper，这样容器内仍可使用配置值 "mysqldump"，
# 但最终会自动转发到系统真实二进制，并补上兼容参数。
#
# wrapper 还要在转发前摘掉 --skip-opt。原因：
# canal（go-mysql）做全量转储时固定会带 --skip-opt，而 MariaDB 的 dump 在这个开关下
# 会额外下发一句
#     SET SQL_QUOTE_SHOW_CREATE=1 /*!40102 ,SQL_MODE=concat(@@sql_mode, _utf8 ,
#     NO_KEY_OPTIONS,NO_TABLE_OPTIONS,NO_FIELD_OPTIONS) */
# 这三个 sql_mode 取值是 MariaDB 独有的，MySQL 服务端不认识，直接报
#     Variable sql_mode can not be set to the value of NO_KEY_OPTIONS (1231)
# 于是 dump 以 exit 2 结束，canal 随之启动失败，ES 投影与图片引用同步一起静默死掉
# （服务本身照常提供 HTTP，所以只有列表是空的，没有任何报错）。
# 上游传什么参数我们改不掉，只能在转发前摘掉这一个开关。
# 摘掉后实测：6 张表全量 dump 退出码 0，输出只剩 USE / INSERT / CHANGE MASTER 三类
# 语句，正是 canal 解析器期望的形状。
RUN printf '%s\n' \
	'#!/bin/sh' \
	'set -eu' \
	'' \
	'# 摘掉 canal 固定传入、但会触发 MariaDB 下发不兼容 sql_mode 的 --skip-opt。' \
	'for arg in "$@"; do' \
	'	shift' \
	'	if [ "$arg" = "--skip-opt" ]; then' \
	'		continue' \
	'	fi' \
	'	set -- "$@" "$arg"' \
	'done' \
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
