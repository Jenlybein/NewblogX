# blogx

一个可以自己部署的博客 / 社区系统：Go 写的 API，两个独立的 Nuxt 前端（读者站、管理后台），外加 MySQL 主从、Redis、Elasticsearch、ClickHouse 这一套基础设施。

**这不是 monorepo**——仓库里没有 workspace、没有 turbo、没有共享代码包。三个应用各自装依赖、各自构建，唯一的耦合点是后端生成的 OpenAPI 契约（见[契约](#七前后端契约唯一耦合点)）。

**只想尽快跑起来？** 直接跳到[第一章：快速开始](#一快速开始)——本地、服务器 HTTPS、服务器无域名三种场景，各一套复制粘贴的命令。

---

## 目录

- [一、快速开始](#一快速开始)
- [二、它由什么组成](#二它由什么组成)
- [三、请求是怎么流向的](#三请求是怎么流向的)
- [四、环境准备](#四环境准备)
- [五、配置](#五配置)
- [六、本地开发](#六本地开发)
- [七、前后端契约（唯一耦合点）](#七前后端契约唯一耦合点)
- [八、后端命令行参考](#八后端命令行参考)
- [九、门禁（提交前必跑）](#九门禁提交前必跑)
- [十、部署到服务器](#十部署到服务器)
- [十一、运维手册](#十一运维手册)
- [十二、代码约定（改代码前请读）](#十二代码约定改代码前请读)
- [十三、常见问题](#十三常见问题)
- [十四、需要时可查的文件](#十四需要时可查的文件)

---

## 一、快速开始

照你的情况选一条，全程复制粘贴。

| 场景 | 用在什么时候 | 起来之后访问 |
| --- | --- | --- |
| **1. 本地 Docker** | 在自己电脑上看效果、改样式 | `http://localhost` |
| **2. 服务器 HTTPS** | 域名已备案、有证书，正式对外 | `https://你的域名` |
| **3. 服务器无域名** | 备案审核中，或只想先验证功能 | `http://服务器IP` |

三条路的开头一样：装 Docker、取代码、把密钥填进 `.env`。差别只在"站点地址"那几个变量，以及要不要证书。

### 准备（三条路都做）

```bash
# 装 Docker（已装可跳过）
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker "$USER" && newgrp docker
docker compose version                  # 输出 v2.x 才继续

# 取代码，进入编排目录
git clone <仓库地址> blogx && cd blogx/deploy/compose/production

# 生成 .env 并填好所有密钥（这一段与域名无关）
cp env.example .env
rand() { openssl rand -base64 24 | tr -d '/+=' ; }
DBPASS="$(rand)"
sed -i \
  -e "s|^BLOGX_JWT_SECRET=.*|BLOGX_JWT_SECRET='$(openssl rand -base64 36 | tr -d '/+=')'|" \
  -e "s|^BLOGX_JWT_ISSUER=.*|BLOGX_JWT_ISSUER='blogx'|" \
  -e "s|^BLOGX_DB_PASSWORD=.*|BLOGX_DB_PASSWORD='$DBPASS'|" \
  -e "s|^BLOGX_DB_REPL_PASSWORD=.*|BLOGX_DB_REPL_PASSWORD='$(rand)'|" \
  -e "s|^BLOGX_RIVER_MYSQL_PASSWORD=.*|BLOGX_RIVER_MYSQL_PASSWORD='$DBPASS'|" \
  -e "s|^BLOGX_IMAGE_REF_RIVER_MYSQL_PASSWORD=.*|BLOGX_IMAGE_REF_RIVER_MYSQL_PASSWORD='$DBPASS'|" \
  -e "s|^BLOGX_REDIS_USERNAME=.*|BLOGX_REDIS_USERNAME=''|" \
  -e "s|^BLOGX_REDIS_PASSWORD=.*|BLOGX_REDIS_PASSWORD='$(rand)'|" \
  -e "s|^BLOGX_ES_PASSWORD=.*|BLOGX_ES_PASSWORD='$(rand)'|" \
  -e "s|^BLOGX_CLICKHOUSE_PASSWORD=.*|BLOGX_CLICKHOUSE_PASSWORD='$(rand)'|" \
  -e "s|^BLOGX_LOGIN_EMAIL_ENABLED=.*|BLOGX_LOGIN_EMAIL_ENABLED='false'|" \
  -e "s|^BLOGX_AI_ENABLED=.*|BLOGX_AI_ENABLED='false'|" \
  .env
```

几条注意事项：

- `BLOGX_REDIS_USERNAME` 必须留空（Redis 只配了 `requirepass`，没有 ACL 账号），上面已经处理。
- 两个 `*_RIVER_MYSQL_PASSWORD` 必须等于 `BLOGX_DB_PASSWORD`。填错**不报错**，只表现为"搜索永远搜不到新文章"。
- 剩下的 `******` 只有邮件、QQ 登录、七牛凭据，用不到就保持关闭（模板默认已关）。
- 首次启动要构建 ES 分词插件和三个应用镜像，约 5~10 分钟，之后不再等。

### 1. 本地 Docker 拉起

在自己电脑上跑（Windows / macOS 的 Docker Desktop，或 Linux 的 Docker）。**不用改任何配置**——模板默认就是 `local` 形态、`dev` 环境、`localhost` 地址。

```bash
# Linux 本机需要放开 ES 要求的内核参数；Docker Desktop 一般不用管
sudo sysctl -w vm.max_map_count=262144

docker compose up -d --build
docker compose ps                        # 等全部 healthy
```

访问 `http://localhost`（读者站）与 `http://localhost/admin/`（后台，**注意尾斜杠**）。

再建一个管理员——系统没有默认账号：

```bash
docker compose exec blogx_server /app/server -f config/settings.yaml \
  -t user -s create -user-role admin -user-username admin -user-password '换成你的密码'
```

数据落在 `deploy/state/`：`docker compose down` 只是停止，数据还在；想彻底重置就 `down` 之后删掉这个目录。

### 2. 服务器 HTTPS Docker 拉起

前提：域名已备案并解析到这台机器，证书已拿到，安全组放开 80 / 443，内存 8 GB 起。

**改 `.env`**（把域名换成你的）：

```bash
DOMAIN=blog.example.com
sed -i \
  -e "s|^BLOGX_NGINX_ENV=.*|BLOGX_NGINX_ENV='production'|" \
  -e "s|^BLOGX_APP_ENV=.*|BLOGX_APP_ENV='prod'|" \
  -e "s|^BLOGX_WEB_ENV_PROFILE=.*|BLOGX_WEB_ENV_PROFILE='production'|" \
  -e "s|^BLOGX_ADMIN_ENV_PROFILE=.*|BLOGX_ADMIN_ENV_PROFILE='production'|" \
  -e "s|^BLOGX_WEB_SITE_URL=.*|BLOGX_WEB_SITE_URL='https://$DOMAIN'|" \
  -e "s|^BLOGX_ADMIN_SITE_URL=.*|BLOGX_ADMIN_SITE_URL='https://$DOMAIN/admin'|" \
  -e "s|^BLOGX_ADMIN_WEB_SITE_URL=.*|BLOGX_ADMIN_WEB_SITE_URL='https://$DOMAIN'|" \
  .env
```

`BLOGX_APP_ENV` 写的是 `prod` 而不是 `production`，这不是笔误：代码里唯一的判断是 `Env == "prod"`，只有它会给 refresh Cookie 加 `Secure`。

**放证书、起服务**：

```bash
sudo sysctl -w vm.max_map_count=262144

mkdir -p ../../state/nginx/cert
cp 你的证书.pem ../../state/nginx/cert/domain.pem
cp 你的证书.key ../../state/nginx/cert/domain.key
chmod 600 ../../state/nginx/cert/domain.key

docker compose up -d --build
docker compose exec blogx_server /app/server -f config/settings.yaml \
  -t user -s create -user-role admin -user-username admin -user-password '换成你的密码'
```

访问 `https://你的域名` 与 `https://你的域名/admin/`。

域名和 `render-config.sh` 里的默认值（`blogx.gentlybeing.cn`）不同也能跑——nginx 只有一个 server 块，会响应任意 Host；但**证书必须与域名匹配**，否则浏览器报警告。想彻底对齐就改 `deploy/docker/nginx/render-config.sh` 里 production 段的 `BLOGX_NGINX_SERVER_NAME`。

### 3. 服务器无域名 Docker 拉起

域名还在备案、或者根本没有域名时用这条：**公网 IP + http**，能验证绝大部分功能。

前提：安全组放开 80，内存 8 GB 起。

**改 `.env`**（把 IP 换成你的公网 IP）：

```bash
IP=1.2.3.4
sed -i \
  -e "s|^BLOGX_NGINX_ENV=.*|BLOGX_NGINX_ENV='local'|" \
  -e "s|^BLOGX_APP_ENV=.*|BLOGX_APP_ENV='dev'|" \
  -e "s|^BLOGX_WEB_SITE_URL=.*|BLOGX_WEB_SITE_URL='http://$IP'|" \
  -e "s|^BLOGX_ADMIN_SITE_URL=.*|BLOGX_ADMIN_SITE_URL='http://$IP/admin'|" \
  -e "s|^BLOGX_ADMIN_WEB_SITE_URL=.*|BLOGX_ADMIN_WEB_SITE_URL='http://$IP'|" \
  .env
```

`BLOGX_APP_ENV` 必须是 `dev`：写 `prod` 会给 Cookie 加 `Secure`，而 http 下浏览器不保存它，表现是"能登录，刷新后掉登录"。

```bash
sudo sysctl -w vm.max_map_count=262144
docker compose up -d --build
docker compose exec blogx_server /app/server -f config/settings.yaml \
  -t user -s create -user-role admin -user-username admin -user-password '换成你的密码'
```

访问 `http://你的公网IP` 与 `http://你的公网IP/admin/`。

能用的：读者站、后台、搜索、评论、聊天、发文。**受影响的只有依赖域名的部分**——QQ 登录回跳、七牛上传回调（多数场景要求域名，图片上传可能暂时不可用）、邮件正文里的链接（用 `site_url` 拼，填 IP 也能跳）。

备案下来后，把上面那段 sed 换成场景 2 的（`production` / `prod` / `https://域名`），放好证书再 `docker compose up -d`，账号和数据都还在。

> 别把未备案的域名解析到大陆机房的 IP 上对外开 80/443——云厂商会阻断并要求整改，这是审核期最常见的停机原因。想先看域名效果，只能在本机 `hosts` 里指向 IP（仅本机生效），或把服务器放到香港 / 境外。

---

## 二、它由什么组成

| 目录 | 是什么 | 技术栈 | 默认端口 |
| --- | --- | --- | --- |
| `backend/` | HTTP API + 后台 worker | Go 1.25 · Gin · GORM | 8080 |
| `frontend/` | 读者站（文章、搜索、创作台、聊天） | Nuxt 4 · Vue 3 · Naive UI · Pinia | 3000 |
| `frontend_admin/` | 管理后台，挂在 `/admin/` | Nuxt 4 · Vue 3 · Naive UI | 3001 |
| `deploy/` | Dockerfile / compose / nginx 模板 | Docker Compose v2 | 80 / 443 |
| `scripts/` | 本地门禁脚本 | Bash | — |
| `frontend*/env/` | 前端按 profile 分文件的环境变量 | — | — |

运行期依赖五个基础设施：**MySQL 8.0**（一主一从，读写分离）、**Redis 7.4**（缓存、计数、限流、会话）、**Elasticsearch 7.12**（文章搜索，装了 IK 中文分词）、**ClickHouse 25.3**（结构化日志库）。

---

## 三、请求是怎么流向的

线上只有 nginx 一个入口，其余容器都不对外暴露端口：

```
浏览器
  │
  ▼
nginx :80 / :443          ← 唯一入口，其余容器都不对外暴露端口
  ├── /             → blogx-web   :3000   读者站（SSR）
  ├── /admin/       → blogx-admin :3001   管理后台（SPA）
  ├── /api/         → blogx-server:8080   HTTP API
  ├── /api/chat/ws  → blogx-server:8080   WebSocket（聊天）
  └── /uploads/     → blogx-server:8080   静态图片

blogx-server（Go）
  ├── MySQL 主库 ──binlog(river)──► Elasticsearch   （写作落 MySQL，搜索走 ES）
  ├── MySQL 从库                                     （读请求，读写分离）
  ├── Redis                                          （缓存 / 计数 / 限流 / 会话）
  └── 日志 ──批量──► ClickHouse                      （失败落 logs/wal，恢复后重放）
```

两条**异步链路**值得单独说明，因为它们决定了"数据不一致时该看哪"：

1. **MySQL → ES（river）**：进程假装成一个 MySQL 从库，读 binlog 把文章/用户/分类等表同步到 ES。写作走 MySQL，搜索走 ES，中间有延迟是正常的。
2. **日志 → ClickHouse**：日志先进有界内存队列，批量写 ClickHouse；写失败落 `logs/wal` 本地兜底目录，恢复后自动重放。所以日志"晚几秒出现"不是故障。

另外有一个**事务型发件箱**（`outbox_event_models`）：领域事件先落库再投递，保证"业务写成功、事件一定不会丢"。它独立于 river 运行。

---

## 四、环境准备

| 工具 | 版本要求 | 说明 |
| --- | --- | --- |
| Go | 1.25+ | `go.mod` 声明 1.25.1 |
| Node.js | 20.12+（建议 22） | 两个前端 `engines` 要求 `>=20.12.0` |
| pnpm | **10.33.0** | 用 `corepack` 锁定，不要用系统全局版本 |
| Docker | 24+ | 本地基础设施与部署都用它 |
| Docker Compose | v2（`docker compose`，不是 `docker-compose`） | — |

前端包管理器**必须走 corepack**。原因不是洁癖：`package.json` 里写死了 `packageManager: pnpm@10.33.0`，Windows 上系统级 pnpm（12.x）装依赖时不建符号链接，会得到一堆找不到模块的报错。

```bash
corepack enable
corepack prepare pnpm@10.33.0 --activate
```

> **Windows 额外注意**：后端测试用 sqlite，需要 CGO，因此需要 gcc（`scoop install mingw`）。
> 跑门禁时脚本会自动把 Go 和 mingw 拉进 PATH，但**必须带 `CGO_ENABLED=1`**，否则后端测试全是红的。

---

## 五、配置

配置分三层，每层管的事情不一样。搞清楚这三层，比死记变量名有用得多。

### 5.1 三层配置的分工

```
第一层  BLOGX_* 环境变量         ← 密钥、地址、开关，随环境变化
            │
            ▼  占位符替换（${VAR}）
第二层  backend/config/settings.yaml     ← 进程级配置：库、缓存、日志、对象存储
        backend/config/site_default_settings.yaml ← 站点展示类默认值
            │
            ▼  首次启动写入数据库
第三层  数据库里的运行时配置      ← 后台改站点设置 / AI 设置，改的是这里，以此为准
```

关键点：

- **`settings.yaml` 里全是 `${BLOGX_*}` 占位符**。变量没设，进程启动时会直接 panic 并告诉你缺哪几个——这是特性，不是 bug，避免带着半套配置跑起来。
- **`site_default_settings.yaml` 只在数据库还没有运行时配置记录时作为初始值写入**。之后在后台改过的值以数据库为准；想回到默认值，得清掉对应的运行时配置记录。
- **`.env` / `.envrc` 都不进仓库**（`.gitignore` 已排除），模板是 `.envrc.example`（跑在宿主机上，带 `export` 前缀）和 `deploy/compose/production/env.example`（跑在容器里，无前缀）。两份键名一致，**唯一必须不同的一项是 `BLOGX_APP_HOST`**：宿主机用 `127.0.0.1`，容器内必须 `0.0.0.0`。改密码或 JWT 时两处要一起改。

### 5.2 后端必填变量

完整清单见 `deploy/compose/production/env.example`（约 100 项，含注释）。**真正必须自己填的只有下面这些**，其余可以照抄示例值：

| 变量 | 作用 | 不填的后果 |
| --- | --- | --- |
| `BLOGX_JWT_SECRET` | 登录态签名密钥 | 泄露 = 任何人都能伪造登录态。**必须换成高强度随机串** |
| `BLOGX_JWT_ISSUER` | 签发者标识 | 用示例值也能跑，但建议改 |
| `BLOGX_DB_PASSWORD` | MySQL 业务账号密码 | 示例是 `******` |
| `BLOGX_DB_REPL_USER` / `BLOGX_DB_REPL_PASSWORD` | 主从复制账号 | 从库连不上，读写分离失效 |
| `BLOGX_REDIS_PASSWORD` | Redis 密码 | 容器渲染配置时用到 |
| `BLOGX_ES_PASSWORD` | ES `elastic` 用户密码 | 健康检查、river 都会失败 |
| `BLOGX_CLICKHOUSE_PASSWORD` | ClickHouse 密码 | 日志写不进去（会落本地 WAL） |
| `BLOGX_APP_ENV` | `dev` / `test` / **`prod`** | 只认字面量 `prod`：它给 refresh Cookie 加 `Secure`。写 `production` 不会开启 Secure（两个名字不等价）；http 环境必须是 `dev` |
| `BLOGX_GIN_MODE` | `debug` / `release` | 生产用 `debug` 会打印大量 SQL 与调试信息 |
| `BLOGX_APP_HOST` | 监听地址 | **容器内必须是 `0.0.0.0`**（见下方陷阱） |
| `BLOGX_NGINX_ENV` | 部署形态开关，见 [10.2](#102-nginx-的四种形态) | 决定 nginx 是否要求域名与证书 |

还有两条**联动关系**，不填错也不报错，只表现为"某个功能莫名其妙不工作"：

- `BLOGX_RIVER_MYSQL_PASSWORD` 与 `BLOGX_IMAGE_REF_RIVER_MYSQL_PASSWORD` 必须等于 `BLOGX_DB_PASSWORD`（两个 river 都用业务账号连主库读 binlog）。填错的表现：服务一切正常，但**搜索永远搜不到新文章**。
- `BLOGX_REDIS_USERNAME` 必须留空。Redis 侧只配了 `requirepass`，没有 ACL 账号，填任何名字都会认证失败。

生成密钥：

```bash
# 任意一种都行，够长够随机即可
openssl rand -base64 48
head -c 48 /dev/urandom | base64
```

> **`BLOGX_APP_HOST` 是这两份模板里唯一"必须不一样"的值**，也是历史上最容易出错的一处：
> 容器内的进程监听 `127.0.0.1` 时，服务只在容器自己的回环上存在，nginx 经 `blogx_network`
> 访问 `blogx_server:8080` 会被连接拒绝。症状极阴——`/api/` 全部 502，而 `docker compose ps`
> 显示所有容器都 healthy（健康检查是容器内回环发起的，看不出来）。`env.example` 已默认写成
> `0.0.0.0`；`.envrc.example` 保持 `127.0.0.1`，因为那是宿主机上的 `go run`。

### 5.3 第三方服务（按需开启）

不配也能跑，只是对应功能不可用：

| 变量组 | 功能 | 关闭后 |
| --- | --- | --- |
| `BLOGX_SMTP_*` | 邮箱验证码登录 / 注册 | 邮箱登录报错，用户名密码登录仍可用 |
| `BLOGX_QQ_*` + `BLOGX_LOGIN_QQ_ENABLED` | QQ 登录 | 登录页不显示 QQ 入口 |
| `BLOGX_QINIU_*` + `BLOGX_QINIU_ENABLED` | 图片上传（文章配图） | 上传接口返回「站点未启用图片上传」。**没有本地上传兜底**：`/uploads` 只是静态路由，代码里没有写入它的逻辑 |
| `BLOGX_AI_*` + `BLOGX_AI_ENABLED` | 站内 AI 助手（搜索/润色） | AI 相关入口报"未启用" |
| `BLOGX_RIVER_ENABLED` | MySQL → ES 同步 | 搜索功能不可用（索引不会更新） |
| `BLOGX_IMAGE_REF_RIVER_ENABLED` | 图片引用关系同步 | 图片引用统计不准 |

`BLOGX_AI_BASE_URL` 是 **Chat Completions 风格**的完整地址（例如 `https://api.openai.com/v1/chat/completions`），不是 base 域名——填错表现为 404 而不是参数错误。

### 5.4 前端配置：profile 机制

两个前端都不读 `.env`，而是读 `env/<profile>.env`，并叠加 `env/common.env` 的公共默认值。**profile 由 `pnpm dev` / `pnpm build` 脚本注入**：

```bash
cd frontend
pnpm dev                  # = dev:local-local（本地前端 + 本地后端）
pnpm dev:local-test       # 本地前端 + 远程测试后端
pnpm build:test
pnpm build:production
```

| profile | 读者站含义 | 后台含义 |
| --- | --- | --- |
| `local-local` | 前端 dev:3000 + 后端 `127.0.0.1:8080` | 前端 dev:3001 + 后端 `127.0.0.1:8080` |
| `local-test` | 本地前端 + 远程测试环境 | 本地前端 + 远程测试环境 |
| `test` | 构建给 `blog.gentlybeing.cn` | 同左 |
| `production` | 构建给 `blogx.gentlybeing.cn` | 同左 |

`frontend/env/*.env` 里可写的键：

| 键 | 说明 |
| --- | --- |
| `BLOGX_WEB_SITE_URL` | 站点自身对外地址（用于 SEO、分享链接） |
| `BLOGX_WEB_API_UPSTREAM` | 后端地址。dev 时是 vite 代理目标，SSR 时是 Node 侧请求目标 |
| `BLOGX_WEB_API_BASE` | 浏览器侧 API 前缀，默认 `/api` |
| `BLOGX_WEB_WS_PATH` | WebSocket 路径，默认 `/api/chat/ws` |
| `BLOGX_WEB_ASSET_PROXY_BASE` | 图片资源前缀，默认 `/uploads` |

后台额外有 `BLOGX_ADMIN_BASE_URL`（默认 `/admin/`）和 `BLOGX_ADMIN_WEB_SITE_URL`（跳回读者站用）。

> **为什么每份 env 只能写本应用的键？**
> `nuxt.config.ts` 里有一道**前缀白名单**：读者站只接受 `BLOGX_WEB_*` / `NUXT_*`，后台只接受 `BLOGX_ADMIN_*` / `NUXT_*`，其余直接忽略。
> 这来自一个真实事故：env 文件早年从后端配置抄过来，于是 `BLOGX_JWT_SECRET` 被无差别注入了前端进程环境——浏览器拿不到，但服务端日志、错误上报里可能带上。白名单从源头堵住这条路。

> **容器部署时前端不用重新构建。** compose 在**运行期**把 `BLOGX_WEB_*` / `BLOGX_ADMIN_*` 映射成 Nuxt 标准的 `NUXT_*` 变量（见 `deploy/compose/production/docker-compose.yml` 的 `blogx_web` / `blogx_admin` 段），所以同一个镜像可以换环境复用。唯一不能变的是后台的 `/admin/` 路径——它是构建期约束，改了静态资源路径会对不上。

---

## 六、本地开发

有两种玩法，**推荐第二种**：基础设施用 Docker，应用在宿主机跑。改代码能热更新，断点也能打。

### 6.1 第一步：起基础设施

```bash
cd deploy/compose/local
cp ../production/env.example .env    # 端口已按本地口径写好（从库 3307）
# 编辑 .env，至少填 DB / Redis / ES / ClickHouse 的密码
docker compose up -d
```

本地编排会把这些端口映射到宿主机（生产编排不会）：

| 服务 | 宿主机端口 | 用途 |
| --- | --- | --- |
| `mysql-master` | 3306 | 写库 |
| `mysql-slave` | 3307 | 读库（主从已配好 GTID 自动定位） |
| `es` | 9200 | 搜索 |
| `redis` | 6379 | 缓存 |
| `clickhouse` | 8123（HTTP）/ 9000（TCP） | 日志 |

首次启动要下载镜像并编译 ES（要装 IK 分词插件，约几分钟）。等就绪：

```bash
docker compose ps          # 全部 healthy 才继续
docker compose logs -f es  # ES 最后一个就绪，约 40 秒
```

### 6.2 第二步：注入后端环境变量

```bash
cd <仓库根目录>
cp .envrc.example .envrc
# 编辑 .envrc：把 ****** 换成真实值；本地地址已是 127.0.0.1
direnv allow              # 看到 "direnv: export ..." 才算真正注入
```

没装 direnv，也可以手动加载或直接内联：

```bash
set -a && . ./.envrc && set +a
# 或者：direnv exec . go run ./cmd/server
```

### 6.3 第三步：初始化数据库与索引

```bash
cd backend
go run ./cmd/server -f config/settings.yaml -t run -s init
```

`-t run -s init` 做三件事：建表（`schema.sql`，全部 `CREATE TABLE IF NOT EXISTS`，可重复执行）→ 确保 ES 索引与写入流水线存在 → **继续正常启动**。

注意最后那一步：它不退出，会直接在当前终端把服务跑起来。想先初始化再单独控制服务，就 `Ctrl+C` 停掉，之后用 `go run ./cmd/server` 正常启动；只想初始化、不想启动，用下面两条拆开的命令。

也可以拆开做（排查问题时更有用）：

```bash
go run ./cmd/server -f config/settings.yaml -db          # 只建表
go run ./cmd/server -f config/settings.yaml -es -s ensure # 只建索引/流水线
```

### 6.4 第四步：创建管理员账号

**没有默认账号**，第一次必须用命令行创建，否则后台登录不上：

```bash
cd backend
go run ./cmd/server -f config/settings.yaml -t user -s create \
  -user-role admin \
  -user-username admin \
  -user-password 'YourStrongPassword' \
  -user-nickname 站长
```

`-user-role` 接受 `admin` / `user` / `guest`，也接受数字 `1` / `2` / `3`。
不加任何 `-user-*` 参数时进入**交互模式**：逐步问角色、用户名、密码（密码不回显）。

> **别被数字骗了**：后端 `RoleAdmin=1`、`RoleUser=2`、`RoleGuest=3`，而管理后台前端下拉框的显示顺序是「普通用户 / 管理员 / 访客」。
> 这个方向历史上是反的，曾在后台造成"选普通用户、结果设成管理员"的提权事故。现在前端已收口到 `frontend_admin/app/utils/roles.ts` 并有单测钉住数字。**用命令行建号时请用文字角色名，别用数字。**

### 6.5 第五步：启动两个前端

```bash
# 终端 A —— 读者站
cd frontend
corepack pnpm install
corepack pnpm dev            # http://localhost:3000

# 终端 B —— 管理后台
cd frontend_admin
corepack pnpm install
corepack pnpm dev            # http://localhost:3001/admin/
```

### 6.6 访问地址

| 地址 | 是什么 |
| --- | --- |
| http://localhost:3000 | 读者站 |
| http://localhost:3001/admin/ | 管理后台 |
| http://127.0.0.1:8080/health | 后端健康检查 |
| http://127.0.0.1:8080/api/... | API 直连 |
| http://127.0.0.1:9200 | ES（`elastic` / `.envrc` 里的密码） |

注意后台的**尾斜杠**：`/admin` 在 nginx 里是 301 跳到 `/admin/`，本地直连 3001 时也要带 `/admin/`，否则 404。

### 6.7 只想全部跑在 Docker 里？

```bash
cd deploy/compose/production     # 生产编排同样能本地跑
cp env.example .env              # 然后把 BLOGX_NGINX_ENV 改成 local
docker compose up -d --build
# 浏览器打开 http://localhost
```

`BLOGX_NGINX_ENV=local` 时 nginx 不要求证书、只监听 80，两个子站都走容器内 upstream。想省内存就把 `BLOGX_NGINX_ENV` 改成 `api-only`，只留 API。

---

## 七、前后端契约（唯一耦合点）

契约的**唯一来源是后端 Go 代码里的注解**。前端类型全部机器生成，不手写、不跨 app 共享。

```
backend 注解  ──swag v2──►  backend/docs/swagger.json  ──openapi-typescript──►  frontend/app/types/openapi.d.ts
                                                       └─────────────────────►  frontend_admin/app/types/openapi.d.ts
```

**改了接口就要重新生成两遍**，第一遍后端、第二遍每个前端：

```bash
cd backend
go install github.com/swaggo/swag/v2/cmd/swag@v2.0.0-rc6   # 只需一次
make openapi                                              # → docs/swagger.json

cd ../frontend        && corepack pnpm openapi:types       # → app/types/openapi.d.ts
cd ../frontend_admin  && corepack pnpm openapi:types
```

三个坑，都踩过：

1. **必须用 swag v2**，且命令带 `--v3.1`。v1 只出 Swagger 2.0，而 `openapi-typescript` 只吃 3.x。
2. **`make openapi` 失败时 exit code 可能仍是 0**（例如注解里写了 `map[string]any`，swag 报错后静默跳过写文件）。所以**验证成功要看文件时间或 grep 新内容**，不要只看退出码。
3. 注解引用别的包的类型时，**那个包必须被文件真正 import**——注释不参与编译。代码里没用到就写 `_ "路径"`。

前端侧用 `app/utils/contract.ts` 的三层工具消费契约：`RequiredSchema<K>`（响应必填）、`Nullable<T,K>`（显式可空）、`OptionalFields<T,K>`（后端可能不返回）。**可空性有三种状态**，混了就会出现"构造兜底对象时编译不过"：`T` → `T`；`*T` → `T | null`；`*T` + `omitempty` → 字段整个缺席，必须用 `OptionalFields`。

CI 有两个 job 专门盯这件事：`contract`（`swagger.json` 必须与注解一致）和 `contract-types`（两份 `openapi.d.ts` 必须与 `swagger.json` 一致）。**只做前一半会漏掉"改了注解→生成契约→忘了更新前端类型"这种失效。**

---

## 八、后端命令行参考

```
./server [flags]
```

| 参数 | 取值 | 作用 |
| --- | --- | --- |
| `-f` | 路径，默认 `config/settings.yaml` | 配置文件位置 |
| `-role` | `api` / `river` / `image-ref` / `cron` / `worker` / `all` | 进程角色，默认 `all` |
| `-db` | 无 | 执行建表（幂等），做完就退出 |
| `-es` + `-s` | `init` / `ensure` / `delete` / `article-sync` | ES 索引操作，做完就退出 |
| `-t` + `-s` | `run init` / `user create` | 组合操作 |
| `-version` / `-v` | 无 | 打印版本（**在读取配置之前处理**，缺环境变量也能用） |

### role 的含义

| role | 启动什么 | 什么时候单独用 |
| --- | --- | --- |
| `api` | HTTP 服务 + outbox 中继 | 只扩 API 实例、不动后台任务 |
| `river` | MySQL → ES 同步 + outbox 中继 | 把 CDC 与 API 拆到不同机器 |
| `image-ref` | 图片引用关系监听 | 同上 |
| `cron` | 定时任务（Redis 计数刷盘、日志清理） | 需要单点执行的任务 |
| `worker` | river + image-ref + cron | 后台任务全包 |
| `all` | worker + api（默认） | 单机部署 |

**`cron` 与 `river` 类角色要单实例。** river 靠 `BLOGX_RIVER_SERVER_ID` 区分身份，cron 靠 Redis 锁互斥——多实例同时跑不会写坏数据，但会互相抢锁、白白消耗资源。

### ES 子命令

| 命令 | 作用 | 危险度 |
| --- | --- | --- |
| `-es -s ensure` | 索引/流水线不存在就创建，存在不动 | 安全，可重复 |
| `-es -s init` | **先删后建**索引与流水线 | 会清空索引（重建后靠 river 回填） |
| `-es -s delete` | 删除索引与流水线 | 搜索立即不可用 |
| `-es -s article-sync` | 把 MySQL 里的文章全量刷进 ES | 安全，用于索引被清后回填 |

改了 `article_models` 的字段映射之后的标准动作：

```bash
./server -f config/settings.yaml -es -s init          # 重建索引（新映射）
./server -f config/settings.yaml -es -s article-sync  # 全量回填
```

---

## 九、门禁（提交前必跑）

CI 只在你 push 之后才跑，所以本地有这个脚本。**跑过它，等于本地结论和 CI 一致。**

```bash
bash scripts/check.sh            # 全量：后端 + 两个前端 + 门禁自检
bash scripts/check.sh backend    # 只跑后端
bash scripts/check.sh frontend   # 只跑两个前端
bash scripts/check.sh gates      # 只做门禁自检（证明门禁真的会红）
```

它检查的东西：`go build` / `go vet` / 架构守卫棘轮（两遍，第二遍含 `_test.go`）/ 集成测试（`-tags integration -race`）/ 契约类型是否过期 / 两端 `lint` + `typecheck` + `test` + `coverage` 门槛。

可用环境变量：

| 变量 | 效果 |
| --- | --- |
| `BLOGX_FAST=1` | 跳过 `-race`，快约一倍。**提交前别用** |
| `BLOGX_NO_CACHE=1` | 不复用 Go 测试缓存，强制全量真跑（CI 语义） |
| `BLOGX_SKIP_INTEGRATION=1` | 只跑单元测试，跳过 golden 与容器契约（会打印警告） |
| `BLOGX_MYSQL_ADDR=host:port` | 复用已有 MySQL，省掉每次起临时容器（约 10 秒） |

集成测试需要 Docker（testcontainers 会起 MySQL/ClickHouse）。**Docker 不可用时脚本会跳过并打印醒目警告，不会静默通过**——静默跳过正是门禁撒谎的经典方式，这里专门堵过。

几个已知的本地噪声，不是代码问题：

- `nuxt prepare` 偶发 `EPERM: .nuxt/eslint.config.mjs`（Windows 文件被占用），脚本会自动重试三次。
- 架构棘轮基线在 `scripts/arch-baseline.env`，**当前是 0**，只准降不准升。别在别处写死这个数字。

---

## 十、部署到服务器

### 10.1 前置条件

| 项 | 建议 |
| --- | --- |
| 系统 | Linux（x86_64），Docker 24+ 与 Compose v2 |
| 内存 | **8 GB 起步，16 GB 更稳**。compose 里各服务 limits 合计约 9.8 GB（那是上限不是占用） |
| 磁盘 | 40 GB+（MySQL 数据 + ES 索引 + ClickHouse 日志 + 上传图片） |
| 端口 | 只开放 80 / 443，其余一律不开 |
| 域名 | 已解析到服务器 IP，证书已备好 |

Linux 上 ES 还需要（否则容器反复重启，日志里是 `max virtual memory areas vm.max_map_count [65530] is too low`）：

```bash
sudo sysctl -w vm.max_map_count=262144
echo 'vm.max_map_count=262144' | sudo tee -a /etc/sysctl.conf
```

### 10.2 nginx 的四种形态

部署形态由**一个变量** `BLOGX_NGINX_ENV` 决定，`render-config.sh` 据此推导域名、证书、upstream 与子站开关：

| 形态 | 域名 | 443 + 证书 | `/` | `/admin/` |
| --- | --- | --- | --- | --- |
| `local` | `localhost` | 否（只监听 80） | 读者站 | 后台 |
| `test` | `blog.gentlybeing.cn` | 是 | 读者站 | 后台 |
| `production` | `blogx.gentlybeing.cn` | 是 | 读者站 | 后台 |
| `api-only` | `blog.gentlybeing.cn` | 是 | 404 | 404 |

要换域名，改的是 `deploy/docker/nginx/render-config.sh` 里的默认值，或者直接在同名环境变量里覆盖（显式传入优先）。

### 10.3 首次部署

```bash
# 1. 取代码
git clone <repo> /opt/blogx && cd /opt/blogx

# 2. 准备生产环境变量
cd deploy/compose/production
cp env.example .env
vi .env
```

`.env` 里**必须改**的（其余可留示例值）：

```ini
BLOGX_APP_ENV='prod'              # 走 https 才写 prod（开 Secure Cookie）；http 环境写 dev
BLOGX_GIN_MODE='release'
BLOGX_APP_HOST='0.0.0.0'          # ← 最容易漏，漏了 /api 全 502

BLOGX_NGINX_ENV='production'       # ← 模板默认是 local，必须改

BLOGX_JWT_SECRET='<openssl rand -base64 48>'
BLOGX_JWT_ISSUER='blogx'

BLOGX_DB_PASSWORD='<强密码>'
BLOGX_DB_REPL_PASSWORD='<强密码>'
BLOGX_REDIS_PASSWORD='<强密码>'
BLOGX_ES_PASSWORD='<强密码>'
BLOGX_CLICKHOUSE_PASSWORD='<强密码>'

# 前端（运行期注入，不用重新构建镜像）
BLOGX_WEB_ENV_PROFILE='production'
BLOGX_WEB_SITE_URL='https://你的域名'
BLOGX_WEB_API_UPSTREAM='http://blogx_server:8080'
BLOGX_ADMIN_ENV_PROFILE='production'
BLOGX_ADMIN_SITE_URL='https://你的域名/admin'
BLOGX_ADMIN_WEB_SITE_URL='https://你的域名'
BLOGX_ADMIN_API_UPSTREAM='http://blogx_server:8080'
```

> **那几行 `MYSQL_ROOT_PASSWORD="$BLOGX_DB_PASSWORD"` 已经从这个模板里删掉了。**
> 原因：`.env` **不做变量嵌套展开**（compose 只对 compose 文件里的 `${...}` 插值，不解析 `.env` 内部的值）。
> 那几行要么落成字面量 `"$BLOGX_DB_PASSWORD"`，要么被人改成 `${BLOGX_DB_PASSWORD}` 之后依然不生效，
> 只是看起来配上了。compose 本来就直读 `BLOGX_*`（见 `MYSQL_ROOT_PASSWORD: ${BLOGX_DB_PASSWORD}`），
> 从来没读过这些别名。确实需要 `docker run --env-file` 时，在命令行显式传即可。

```bash
# 3. 放证书（production / test / api-only 三种形态都读这个固定名）
mkdir -p /opt/blogx/deploy/state/nginx/cert
cp your-cert.pem /opt/blogx/deploy/state/nginx/cert/domain.pem
cp your-cert.key /opt/blogx/deploy/state/nginx/cert/domain.key
chmod 600 /opt/blogx/deploy/state/nginx/cert/domain.key

# 4. 启动
docker compose up -d --build
```

首次启动会自动初始化，不用手动跑迁移。`deploy/docker/api/entrypoint.sh` 会先查 `runtime_site_config_models` 表：

- **表不存在** → 自动执行建表，等 ES 就绪，再 `-es -s ensure`，然后才真正启动服务；
- **表已存在** → 跳过初始化，直接启动。

这带来一个副作用要知道：**想重新初始化，得先把这张表删掉**（或手动执行 `-db`），否则 entrypoint 永远认为"已经初始化过了"。

```bash
# 5. 确认状态
docker compose ps                       # 全部 Up (healthy)
docker compose logs -f blogx_server     # 看启动日志

# 6. 创建管理员（同本地步骤）
docker compose exec blogx_server /app/server \
  -f config/settings.yaml -t user -s create \
  -user-role admin -user-username admin -user-password '<强密码>'

# 7. 冒烟测试
curl -k https://你的域名/health
```

浏览器访问 `https://你的域名`（读者站）与 `https://你的域名/admin/`（后台）。

### 10.4 更新版本

```bash
cd /opt/blogx
git pull

# 契约变了的话，先在本地（或 CI）重新生成并提交：
#   cd backend && make openapi
#   cd ../frontend && pnpm openapi:types
#   cd ../frontend_admin && pnpm openapi:types

cd deploy/compose/production
docker compose up -d --build          # 只重建有变化的镜像
docker compose ps                     # 确认 healthy
```

数据库不需要单独迁移步骤。`-db` 执行的是 `schema.sql`，全部 `CREATE TABLE IF NOT EXISTS`，重复跑没有副作用——这也和项目的**零 DDL 硬约束**一致：**不会 ALTER 既有表**。

### 10.5 回滚

三个镜像 tag 是变量（`env.example` 里已经写好这三行，默认值与 compose 的缺省一致，所以不改也能跑），这是留给回滚的口子：

```ini
# deploy/compose/production/.env
BLOGX_SERVER_IMAGE='blogx-server:production'
BLOGX_WEB_IMAGE='blogx-web:production'
BLOGX_ADMIN_IMAGE='blogx-admin:production'
```

```bash
# 出事时：把上面三行改成旧 tag，然后
docker compose up -d      # 注意：不要带 --build
```

**为什么不带 `--build` 是重点**：compose 里这几个服务同时有 `build:` 和 `image:`，带 `--build` 会重新构建并把 tag 指回新代码——那等于把"回滚"变成"又发布了一次"，而且看起来还成功了。

所以**发布前给镜像打日期 tag 并保留**，是让回滚真正可行的前提：

```bash
docker tag blogx-server:production blogx-server:2026-10-26
docker tag blogx-web:production    blogx-web:2026-10-26
docker tag blogx-admin:production  blogx-admin:2026-10-26
```

### 10.6 上线检查清单

- [ ] `BLOGX_APP_HOST=0.0.0.0`、`BLOGX_NGINX_ENV=production`、`BLOGX_GIN_MODE=release`
- [ ] 五个密码全部换成强密码，`BLOGX_JWT_SECRET` 已重新生成
- [ ] 证书放在 `deploy/state/nginx/cert/domain.pem` / `domain.key`，私钥权限 600
- [ ] 防火墙只放 80/443；`docker compose ps` 里 mysql/redis/es/clickhouse **没有**宿主机端口映射
- [ ] `curl https://域名/health` 返回 200
- [ ] `/` 与 `/admin/` 都能打开，后台能登录（已用命令行建号）
- [ ] 上传一张图片，确认返回七牛 CDN 域名且能打开。**没配七牛时这一项会报「站点未启用图片上传」，属预期**
- [ ] 发一篇文章，等几秒后在站内搜索能搜到（验证 river 链路）
- [ ] 后台日志页有数据（验证 ClickHouse 链路）
- [ ] 备份任务已配置（见 [11.3](#113-备份与恢复)）

---

## 十一、运维手册

### 11.1 端口与容器

| 容器 | 生产是否暴露 | 说明 |
| --- | --- | --- |
| `nginx` | **80 / 443** | 唯一入口 |
| `blogx-server` | 否 | Go API，`/health` 供健康检查 |
| `blogx-web` | 否 | 读者站 Nitro，3000 |
| `blogx-admin` | 否 | 后台 Nitro，3001 |
| `mysql-master` / `mysql-slave` | 否 | 3306 |
| `redis` | 否 | 6379 |
| `es` | 否 | 9200 |
| `clickhouse` | 否 | 9000 / 8123 |

需要临时连进去排查时，用 `docker compose exec` 而不是改 compose 加端口——改端口意味着把数据库暴露到公网。

### 11.2 常见运维命令

```bash
cd /opt/blogx/deploy/compose/production
set -a; . ./.env; set +a     # 让下面的 $BLOGX_* 可用（不 source 就是空值）

docker compose ps                          # 状态
docker compose logs -f --tail=200 blogx_server
docker compose restart blogx_server        # 重启单个服务
docker compose up -d --build blogx_server  # 只重建后端

# 看应用日志（按日期分目录，路径相对本目录）
ls ../../state/api/logs/

# 查 ES 索引文档数（走容器内，不用把 9200 暴露到公网）
docker compose exec es curl -s -u "elastic:$BLOGX_ES_PASSWORD" \
  'http://127.0.0.1:9200/article_index/_count?pretty'

# ES 有没有挂过 / 重启过（由容器内的看门狗记录，只在状态变化时写）
docker compose logs blogx_server | grep '\[es-watch\]'

# 查表行数
docker compose exec mysql-master mysql -u"$BLOGX_DB_USER" -p"$BLOGX_DB_PASSWORD" \
  "$BLOGX_DB_NAME" -e "SELECT COUNT(*) FROM article_models"
```

日志目录映射（都在 `deploy/state/` 下，已在 `.gitignore` 内）：

| 宿主机路径 | 容器内 | 内容 |
| --- | --- | --- |
| `state/api/logs/` | `/app/logs` | 应用日志，按日期分目录 + `wal/` 兜底 |
| `state/api/uploads/` | `/app/uploads` | 静态路由目录。**当前代码没有写入方**（历史遗留），图片实际存七牛 |
| `state/api/var/` | `/app/var` | river 位点等运行时状态 |
| `state/mysql/*/data` | `/var/lib/mysql` | 数据库文件 |
| `state/es/data` | ES 数据目录 | 索引 |
| `state/clickhouse/data` | CH 数据目录 | 日志表 |
| `state/nginx/logs/` | `/var/log/nginx` | 访问/错误日志 |

**`state/api/var` 不要随便删。** river 的 binlog 位点存在这里，删了会从头重扫，轻则重复同步、重则把旧数据覆盖回去。

### 11.3 备份与恢复

必须备份的只有两样：MySQL、`state/api/var`（river 位点）。`uploads/` 目录当前没有写入方（图片在七牛），除非你手工往里放过文件，否则不用备份。ES 与 ClickHouse 都是**可重建的派生数据**（ES 靠 `article-sync` 回填，日志库丢了就是丢历史日志）。

```bash
cd /opt/blogx/deploy/compose/production
set -a; . ./.env; set +a

# 备份 MySQL
docker compose exec -T mysql-master \
  mysqldump -u"$BLOGX_DB_USER" -p"$BLOGX_DB_PASSWORD" --single-transaction --routines "$BLOGX_DB_NAME" \
  | gzip > /backup/blogx-$(date +%F).sql.gz

# 备份文件与位点
tar czf /backup/blogx-state-$(date +%F).tar.gz \
  ../../state/api/uploads ../../state/api/var
```

恢复时顺序很重要：**先恢复 MySQL，再恢复 `var/`，最后才启动 blogx_server**。反了的话 river 会以一个未来位点启动，增量数据永远补不回来。

### 11.4 日志去哪看

三个地方，用途不同：

1. **标准输出**：`docker compose logs`，受 compose 的 `json-file` 10 MB × 5 限制，只适合看眼前。
2. **ClickHouse**（`blogx_logs` 库）：结构化日志，后台日志页查的就是它。写入失败会先落 `logs/wal`，恢复后自动重放。
3. **本地文件** `state/api/logs/<date>/`：按天分目录，默认保留 7 天（`BLOGX_LOG_RETENTION_DAYS`），每天 03:30 清理。

"日志被悄悄提前删掉"很难事后发现，所以**生产环境建议把这三个值显式写进 `.env`**，别依赖 `settings.yaml` 的默认值：

```ini
BLOGX_LOG_CLEANUP_ENABLED='true'
BLOGX_LOG_RETENTION_DAYS='7'
BLOGX_LOG_CLEANUP_RUN_AT='03:30:00'
```

---

## 十二、代码约定（改代码前请读）

这几条不是风格偏好，是有守卫脚本兜底的硬约束。违反了门禁会红，或者更糟——**不红但线上出错**。

1. **零 DDL。** 不能改、不能删既有表。ES 投影靠 CDC 按列名解析，改列名会让同步静默错位。**加新表可以。**
2. **ID 语义字段必须用 `ctype.ID`**，禁止裸整型。守卫规则 R12 兜底，基线 0。
3. **不要拿 ID 比大小。** `ctype.ID` 底层是字符串，`"10" < "9"` 成立。要比较先 `.Uint64()`。
4. **不要用 `ID(123)` 构造 ID**，用 `NewID`。`go vet` 的 `stringintconv` 只抓这一种写法。
5. **单文件 ≤ 800 行**，两端 eslint `max-lines` 强制。
6. **HTTP 层零 SQL**，后台 handler 不得直接查库。
7. **子包可以依赖父包，父包不能依赖子包**（archcheck R10/R11 盯这个）。
8. 一个业务拆出的多个文件收进子包，别平铺。

注释用中文写。标识符、类型名用英文。

---

## 十三、常见问题

**启动就 panic，说缺环境变量**
说明 `.envrc` / `.env` 没生效。`direnv allow` 之后要看到 `direnv: export ...` 才算注入。只是想知道版本号：`./server -version`（它在读配置之前就返回了）。

**所有 `/api` 请求 502，但容器全是 healthy**
九成是 `BLOGX_APP_HOST` 还是 `127.0.0.1`。容器内必须监听 `0.0.0.0`，否则 nginx 连不上。

**MySQL 从库报 1777，主从起不来**
`my.cnf` 被挂载成 777，MySQL 会**忽略所有用户可写的配置文件**（只打一行警告），于是 `gtid_mode=ON` 静默失效。compose 里已经用 entrypoint 先复制再 `chmod 644` 规避了（Windows 上 Docker Desktop 必踩）。如果你改了自己的挂载方式，这个坑会回来。

**ES 容器反复重启**
先看日志判定是哪一类，三种现象完全不同：

- `AccessDeniedException ... /usr/share/elasticsearch/data/nodes` —— 宿主机上数据目录的属主不是 uid 1000。
  compose 的 `es` 服务已经设了 `TAKE_FILE_OWNERSHIP: 'true'`，官方 entrypoint 会在启动前
  `chown -R 1000:0` 修掉，**正常不该再出现**；真出现了说明这一步本身失败（宿主机挂载只读、NFS
  `root_squash`、磁盘满），`docker compose logs es | head -5` 能看到那一行报错。注意这个变量是按
  「非空」判定的，写 `'false'` 也照样触发，想关掉只能删掉整行。
- `max virtual memory areas vm.max_map_count [...] is too low` —— Linux 上内核参数太小，见 [10.1](#101-前置条件)。
- OOM / `Killed` —— 内存上限小于 512 MB。`ES_JAVA_OPTS` 已经是 `-Xms512m -Xmx512m`，上限再小就会 OOM。

**怎么知道 ES 中途挂过 / 重启过**
`docker compose ps` 看不出来：`restart: always` 会把它自己拉回来，重启完照样 healthy。
`entrypoint.sh` 会在 `blogx_server` 里拉起一个看门狗（`deploy/docker/api/es-watch.sh`），
每 60 秒采一次 `_cluster/health` 和 JVM uptime，**只在状态变化时**写一行，前缀 `[es-watch]`：

```bash
docker compose logs blogx_server | grep '\[es-watch\]'
```

会看到 `检测到 ES 重启：JVM uptime ... -> ...`、`集群状态变化 green -> yellow`、
`ES 不可达` 这类行。uptime 倒退是唯一可靠的“重启过”信号——即使重启正好发生在两次采样之间也能发现。
它只记录、不干预；要主动通知得另接外部监控。

**搜索搜不到文章，但数据库里明明有**
按顺序查：`BLOGX_RIVER_ENABLED` 是否为 `true` → river 日志有没有报 binlog 权限 → 索引是否存在（`-es -s ensure`）→ 手动回填一次（`-es -s article-sync`）。注意写库后在 ES 里能搜到**有几秒延迟是正常的**。

**后台登录报 401 / 打开 `/admin/` 是 404**
401：账号没建，用 `-t user -s create` 建一个。404：少了尾斜杠，或者 `BLOGX_ADMIN_BASE_URL` 与 nginx 的 `location /admin/` 对不上。

**上传图片报「站点未启用图片上传」**
不是 bug，是没配七牛。图片上传只有七牛这一条落地路径（`BLOGX_QINIU_ENABLED` + 四个 key + bucket），
**没有"存本地"的兜底**——`/uploads` 只是静态路由，代码里没有任何写入它的逻辑，
`upload_dir` 配置项是预留字段、未被读取。要用文章配图就先去七牛建空间。

**图片上传成功但页面显示不出来**
先看接口返回的 URL 是不是 CDN 域名（`BLOGX_QINIU_DOMAIN`，要带 `https://`）。浏览器侧只经 nginx 的
`/uploads/` 与后端静态路由走本地文件，CDN 图片是直连的，跟容器无关——所以"容器里看不到图"是正常的。
前端还有 `BLOGX_WEB_ASSET_PROXY_BASE`（默认 `/uploads`）这个前缀，改错会让所有本地图片 404。

**`pnpm install` 之后一堆模块找不到（Windows）**
用了系统级 pnpm。`corepack prepare pnpm@10.33.0 --activate`，然后用 `corepack pnpm ...`。系统 12.x 在 Windows 上不建符号链接。

**本地门禁报 `EPERM: .nuxt/eslint.config.mjs`**
文件被其他进程短暂占用（杀软/索引/编辑器），不是代码问题。脚本会自动重试三次，还不行就单独重跑一次。

**`make openapi` 说成功，但 `swagger.json` 没变化**
swag 遇到不支持的注解（典型是 `map[string]any`）会报错然后**静默跳过写文件，exit code 仍是 0**。看文件修改时间，或者 grep 一下你新加的接口路径。注解里请用具名类型。

**`nuxi typecheck` 在 Windows 上 OOM**
加 `NODE_OPTIONS=--max-old-space-size=8192`。`scripts/check.sh` 已经默认设了。

**端口 3306 / 9200 / 6379 被占**
本地编排会把基础设施端口映射到宿主机（生产编排不映射）。本机已经装了 MySQL/Redis，或者同时起了另一套 compose，就会撞车。改 `deploy/compose/local/.env` 里的 `BLOGX_DB_MASTER_PORT` / `BLOGX_DB_REPLICA_PORT` / `BLOGX_ES_PORT` / `BLOGX_REDIS_PORT` 即可，后端那边对应的 `BLOGX_*_HOST` / `_PORT` 也要跟着改。

---

## 十四、需要时可查的文件

README 只能回答"怎么用"。更细的东西都在代码库里，而且它们是**现行事实**——改了代码它们跟着变，不会过期：

| 想知道 | 看哪里 |
| --- | --- |
| 全部环境变量、默认值、哪些必须改 | `deploy/compose/production/env.example`（带注释的完整清单） |
| 后端每个配置项什么意思 | `backend/config/settings.yaml`（逐字段中文注释） |
| 站点标题 / 备案号 / 登录开关的默认值 | `backend/config/site_default_settings.yaml` |
| 表结构 | `backend/internal/platform/mysql/ddl/schema.sql` |
| 接口清单与出入参结构 | `backend/docs/swagger.json`，或前端 `app/types/openapi.d.ts` |
| nginx 的路由与反代规则 | `deploy/docker/nginx/nginx.conf.tmpl`；变量推导在 `render-config.sh` |
| 容器编排、依赖顺序、资源上限 | `deploy/compose/production/docker-compose.yml` |
| 门禁到底检查了什么 | `scripts/check.sh` 顶部注释（每个检查项都写了为什么） |
| 架构守卫规则与当前基线 | `backend/tools/archcheck/`、`scripts/arch-baseline.env` |
| CI 有哪些 job、各自拦什么 | `.github/workflows/ci.yml` |

改代码前最该看的一处不是文档，是 `schema.sql` 和 `archcheck` 的规则——前者告诉你哪些字段不能动，后者告诉你哪些写法会被门禁拦下。
