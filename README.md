# blogx

一个可以自己部署的博客 / 社区系统：Go 写的 API，两个独立的 Nuxt 前端（读者站、管理后台），外加 MySQL 主从、Redis、Elasticsearch、ClickHouse 这一套基础设施。

---

## 一、快速开始

可以选择让 agent 使用 ssh 的形式控制你的服务器，快速自动部署这个项目，并指引你去配置好各项配置。

如果你想自己动手部署，则可以看以下指导内容进行部署。

### 准备工作

这次准备工作的允许环境为 `Ubutu 22.04 LTS`

#### 装 Docker（已装可跳过）

```bash
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker "$USER" && newgrp docker
docker compose version                  # 输出 v2.x 才继续
```

#### 拉取代码仓库，进入编排目录

```bash

git clone https://github.com/Jenlybein/NewblogX.git blogx
cd blogx/deploy/compose/production
```

#### 生成配置文件

生成 .env 并填好所有密钥，这里可以修改各种密钥，默认是随机生成（数据库、redis、es、clickhouse）

```bash
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
  .env
```

几条注意事项：

- `BLOGX_REDIS_USERNAME` 必须留空（Redis 只配了 `requirepass`，没有 ACL 账号），上面已经处理。
- 两个 `*_RIVER_MYSQL_PASSWORD` 必须等于 `BLOGX_DB_PASSWORD`。填错**不报错**，只表现为"搜索永远搜不到新文章"。

### 三条拉起服务的路

三条路的开头一样：装 Docker、取代码、把密钥填进 `.env`。差别只在"站点地址"那几个变量，以及要不要证书。

照你的情况选一条路，全程复制粘贴。

| 场景                | 用在什么时候                 | 起来之后访问       |
| ------------------- | ---------------------------- | ------------------ |
| **1. 本地 Docker**  | 在自己电脑上看效果、改样式   | `http://localhost` |
| **2. 服务器 HTTPS** | 域名已备案、有证书，正式对外 | `https://你的域名` |
| **3. 服务器无域名** | 备案审核中，或只想先验证功能 | `http://服务器IP`  |

### 1. 本地 Docker 拉起

在自己电脑上本地运行（Windows / macOS 的 Docker Desktop，或 Linux 的 Docker）。

**不用改任何配置**——模板默认就是 `local` 形态、`dev` 环境、`localhost` 地址。

```bash
# Linux 本机需要放开 ES 要求的内核参数；Docker Desktop 一般不用管
sudo sysctl -w vm.max_map_count=262144

docker compose up -d --build
docker compose ps                        # 等全部 healthy
```

等全部容器 healthy 之后，就可以：

- 访问 `http://localhost`（读者站）；
- 访问 `http://localhost/admin/`（后台，**注意尾斜杠**）。

数据落在 `deploy/state/`，想彻底重置就 `down` 之后删掉这个目录。

### 2. 服务器 HTTPS 拉起 Docker

前提：域名已备案，并解析到这台机器。服务器端开放 80 / 443 接口，内存 4 GB 起。

#### 修改配置文件

改 `.env`，以下命令中修改 `DOMAIN` 的值，把域名换成你的：

```bash
DOMAIN=写你的域名

sed -i \
  -e "s|^BLOGX_NGINX_ENV=.*|BLOGX_NGINX_ENV='production'|" \
  -e "s|^BLOGX_APP_ENV=.*|BLOGX_APP_ENV='production'|" \
  -e "s|^BLOGX_WEB_ENV_PROFILE=.*|BLOGX_WEB_ENV_PROFILE='production'|" \
  -e "s|^BLOGX_ADMIN_ENV_PROFILE=.*|BLOGX_ADMIN_ENV_PROFILE='production'|" \
  -e "s|^BLOGX_WEB_SITE_URL=.*|BLOGX_WEB_SITE_URL='https://$DOMAIN'|" \
  -e "s|^BLOGX_ADMIN_SITE_URL=.*|BLOGX_ADMIN_SITE_URL='https://$DOMAIN/admin'|" \
  -e "s|^BLOGX_ADMIN_WEB_SITE_URL=.*|BLOGX_ADMIN_WEB_SITE_URL='https://$DOMAIN'|" \
  .env
```

`BLOGX_APP_ENV` 写 `production`：代码里唯一的判断就是它，只有生产环境会给 refresh Cookie 加 `Secure`。

#### 放证书

nginx 只认 `deploy/state/nginx/cert/` 下的两个固定文件名：`domain.pem`（**fullchain**，含中间证书）和 `domain.key`（私钥）。证书来源二选一，放好后执行同一段「起服务」。

##### A. 自有证书（云厂商签发 / 已有 pem + key）

这个方法适合你自己已经有证书了，只需要把证书放进对应位置即可：

```bash
mkdir -p ../../state/nginx/cert
cp 你的证书.pem ../../state/nginx/cert/domain.pem
cp 你的证书.key ../../state/nginx/cert/domain.key
chmod 600 ../../state/nginx/cert/domain.key
```

证书的 SAN 必须覆盖你要访问的域名（`https://你的域名`），否则浏览器报"证书与域名不匹配"。

还有，需要自行维护证书是否过期！

##### B. 免费证书 + 自动续期

适合还没有证书、想免费部署的用户。

仓库里带了签发脚本 `scripts/issue-cert.sh`，在仓库根目录用**部署用户**执行。

邮箱和域名用参数传，多个域名就重复 `--domain`（第一个是主域名）：

```bash
# 一般写法
bash scripts/issue-cert.sh --email 你的邮箱地址 --domain 你的域名
# 多域名写法
bash scripts/issue-cert.sh --email 你的邮箱地址 --domain 你的域名 --domain www.你的域名
```

这个脚本会先装 `acme.sh`，再用 `Let's Encrypt` 签发免费证书；之后由 acme.sh 的每日定时任务检查、到期前自动续期。

证书签发时会暂停 nginx 腾出 80 接口，签完后再重新拉起来。后续续期由 cron 负责，不用再跑脚本。

**注意**：

- *续期时*：自动任务检查到证书过期时间小于 30 天时，会自动停 nginx → 签发 → 起 nginx（这几秒访问网站会失败）→ 重载新证书。这种情况出现，是因为脚本采用的是 HTTP-01 的方法。完全不想停 nginx 就请自行改用 DNS-01 方法。
- *签发前必须*：域名的 A/AAAA 解析指向这台服务器的公网 IP、公网 80 放行、域名没有被 CDN/代理（如 Cloudflare 橙云）挡住 HTTP-01 校验。**脚本不会替你改 DNS！**

#### 起服务

首先构建容器：

```bash
sudo sysctl -w vm.max_map_count=262144

docker compose up -d --build
```

容器构建完成后，设置初始管理员账号：

```bash
MYID='换成你的账号名'
MYPSD='换成你的密码'

docker compose exec blogx_server /app/server -f config/settings.yaml \
  -t user -s create -user-role admin -user-username $MYID -user-password $MYPSD
```

等全部容器 healthy 之后，就可以：

- 访问 `https://你的域名`（读者站）；
- 访问 `https://你的域名/admin/`（后台，**注意尾斜杠**）。

### 3. 服务器无域名 Docker 拉起

域名还在备案、或者根本没有域名时用这条：**公网 IP + http**，能验证绝大部分功能。

#### 改配置文件

**改 `.env`**，把 IP 换成你的公网 IP：

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

`BLOGX_APP_ENV` 必须是 `dev`：写 `production` 会给 Cookie 加 `Secure`，而 http 下浏览器不保存它，表现是"能登录，刷新后掉登录"。

首先构建容器：

```bash
sudo sysctl -w vm.max_map_count=262144
docker compose up -d --build
```

容器构建完成后，设置初始管理员账号：

```bash
MYID='换成你的账号名'
MYPSD='换成你的密码'

docker compose exec blogx_server /app/server -f config/settings.yaml \
  -t user -s create -user-role admin -user-username $MYID -user-password $MYPSD
```

等全部容器 healthy 之后，就可以：

- 访问 `http://你的公网IP`（读者站）；
- 访问 `http://你的公网IP/admin/`（后台，**注意尾斜杠**）。

能用的：读者站、后台、搜索、评论、聊天、发文。**受影响的只有依赖域名的部分**——QQ 登录回跳、七牛上传回调（多数场景要求域名，图片上传可能暂时不可用）、邮件正文里的链接（用 `site_url` 拼，填 IP 也能跳）。

备案下来后，把上面那段 sed 换成场景 2 的（`BLOGX_NGINX_ENV` 与 `BLOGX_APP_ENV` 都写 `production`，站点 URL 用 `https://域名`），放好证书再 `docker compose up -d`，账号和数据都还在。

---

## 二、快速配置服务

服务起来之后，"站点能不能用"的开关基本都不在 `.env` 里，而在**管理后台**（地址：`https://你的域名/admin/`）：

- 入口：左侧「站点配置」页，八个 Tab：**站点 / AI 助手 / 邮件 / 对象存储 / 第三方登录 / 日志 / 上传策略 / 装饰**
- 改完点各自的「保存」，**下一次请求就生效，不用重启容器**

这一章只走最短路径：管理员 → 站点信息 → AI → 七牛 → 邮件。

### 2.1 第一次进后台：创建管理员

如果你一开始跳过了创建默认管理员那一步，就会**没有默认账号**。这时，第一次打开 `/admin/` 会被跳到 `/setup`：

1. 填用户名、密码（**至少 8 位**）、可选邮箱（用于找回密码）；
2. 点「创建并进入登录页」，再用刚才的账号登录。

这个入口**只开放一次**：库里一旦有管理员（判断只看角色，哪怕账号被禁用），`/setup` 就永久关闭。

> 如果你是按前面「快速开始」用命令行建的号，`/setup` 已经关闭，直接登录即可。

### 2.2 站点配置

这一页的三组设置（基础信息 / 登录策略 / 内容审核）属于同一段配置，**改完点一次「保存站点配置」**即可：

| 组 | 字段 | 怎么填 |
| --- | --- | --- |
| 基础信息 | 站点标题、Logo URL、备案号、站点模式、SEO 关键词 / 描述 | 站点标题出现在页头与浏览器标题；Logo 可填 `https://...` 或 `/uploads/...`；站点模式按定位选「社区模式 / 博客模式」；SEO 关键词用英文逗号分隔，描述给搜索引擎和分享卡片用 |
| 登录策略 | 用户名密码登录、邮箱验证码登录、QQ 登录、图形验证码；验证码有效期；发码限流；登录失败锁定 | **先配好对应服务再打开开关**：邮箱登录要先配「邮件」Tab，QQ 登录要先配「第三方登录」Tab。开了没配，用户点下去只会看到报错。数值类默认值够用 |
| 内容审核 | 文章免审核、评论免审核 | 打开后发布 / 评论直接过，不进后台审核队列；单人博客建议都打开 |

新手最少要做的：填站点标题与 SEO，按需打开「文章免审核」。邮箱 / QQ 登录留到后面配完服务再开。

### 2.3 AI 助手（「AI 助手」Tab）

AI 是**两层开关**，只开一层不生效：

1. **添加模型配置**（页面下半部分）：点「添加配置」→ 选供应商 → 填模型与密钥 → 保存；
2. 在那条配置上点「**启用**」——同一时刻只有一条生效；
3. 回到上半部分打开「**启用 AI 助手**」总闸，顺手填昵称 / 头像 / 简介。

站点级参数（上半部分）可以修改模型的使用配置：请求超时、最大输入字符、每日配额（`0` = 不限）、采样温度。

**密钥保存后显示 `******`，原样保存 = 不修改。**

验证：打开读者站，点右下角的**捏捏（Jelly）**问一句，能流式回答就通了；后台「数据概览」的 AI token 消耗、日志中心「AI 调用」Tab 也能看到调用记录。

### 2.4 七牛云对象存储（「对象存储」Tab）

图片上传**只有七牛这一条路径**（没有本地兜底），所以要用文章配图必须先配它。控制台侧一次做完：

1. 建空间，记下空间名与**区域**；空间访问控制设**公开读**；
2. 「密钥管理」里拿 AK / SK；
3. 给空间绑一个加速域名（如 `image.example.com`）：按七牛给的 CNAME 去 DNS 加记录，**回七牛点「验证」**——只加解析不点验证，域名不生效；
4. 后台「对象存储」Tab 填：

| 字段 | 填什么 |
| --- | --- |
| 启用七牛云存储 | 开 |
| AccessKey / SecretKey | 七牛 AK / SK。SK 保存后显示 `******`，**留 `******` = 不修改** |
| Bucket | 空间名（如 `myblogx`） |
| 访问域名 | `https://image.example.com`（**带协议头**，且域名得真的配了 HTTPS） |
| 存储区域 | 与空间区域一致；选错不报错，只表现为传不上去 |
| 对象 key 前缀 | 默认 `myblogx`，**别改**：图片引用关系靠它从 URL 反解 object key |
| 单图上限 / 凭证有效期 | 默认 5 MB / 1800 秒；有效期别低于上传耗时，慢网大图可调大（过期会回 401） |
| 上传回调地址 | `https://你的域名/api/images/qiniu/callback`，**必须 HTTPS 且证书覆盖该域名**；留空 = 不启用回调，上传任务会一直 pending |

保存即生效（运行时配置现读，不用重启）。验证：后台「媒体运营」上传一张图，能显示且直链能打开就通了；文章封面 / 配图走同一条链路。

#### 2.4.1 七牛云域名配置 HTTPS

注意，如果你的网站使用了 HTTPS，那么图片链接就无法再使用 HTTP——浏览器会强制升级到 HTTPS（否则会被 mixed content 拦掉）。

所以，七牛云的图片链接域名在这种情况下，最终要配证书。

此时无法使用 HTTP-01 的方式来配置证书（它 CNAME 到七牛，HTTP-01 的验证文件落不到你的服务器）。

只能用 **DNS-01** 方式：先改开头的四个变量（你的邮箱、七牛云获取的 AK/SK、你在七牛云设置的第三方域名），再执行下面三步，之后续期全自动：

```bash
MYMAIL=你的邮箱
MYDOMAIN=你七牛云设置的第三方域名
MY_AK=七牛云获取的AK
MY_SK=七牛云获取的SK

# 0) 没装 acme.sh 先装（已装过跳过）
curl https://get.acme.sh | sh -s email=$MYMAIL

# 1) DNS-01 签发；插件与 API 凭据见第十六章 16.5（示例用火山引擎，其它服务商换对应 --dns 插件）
~/.acme.sh/acme.sh --issue --dns dns_volcengine -d $MYDOMAIN --keylength 2048

# 2) 凭据与证书路径放一个文件里（不进 git，权限 600）
mkdir -p /etc/blogx/qiniu
cat > /etc/blogx/qiniu/env <<'EOF'
QINIU_AK=$MY_AK
QINIU_SK=$MY_SK
CDN_DOMAIN=$MYDOMAIN
KEY_FILE=/etc/blogx/qiniu/image.key
CHAIN_FILE=/etc/blogx/qiniu/image.pem
EOF
chmod 600 /etc/blogx/qiniu/env

# 3) 落盘 + 立刻推给七牛并绑定；以后每次续期都会自动重跑这条 --reloadcmd
~/.acme.sh/acme.sh --install-cert -d $MYDOMAIN \
  --key-file       /etc/blogx/qiniu/image.key \
  --fullchain-file /etc/blogx/qiniu/image.pem \
  --reloadcmd      'set -a; . /etc/blogx/qiniu/env; bash /opt/blogx/scripts/qiniu-cert-sync.sh'
```

路径里的 `/opt/blogx` 换成服务器上仓库的实际目录。

### 2.5 邮件（腾讯云 SES，「邮件」Tab）

邮箱验证码登录、注册、找回密码、绑定邮箱都靠它。腾讯云 SES **只支持模板发信**，先在控制台准备：

1. 开通 SES，验证**发信地址**（如 `noreply@yourdomain.com`）；
2. 建一个验证码模板，变量用 `action` / `code` / `timeout`——注册、重置密码、绑定邮箱、邮箱登录四个场景共用这一个模板；
3. 拿 API 密钥（建议用只授予 SES 权限的子账号）：SecretId / SecretKey；
4. 后台「邮件」Tab 填：SecretId、SecretKey、地域（只支持**广州** `ap-guangzhou` / **中国香港** `ap-hongkong`）、发信地址、发件人显示名、**验证码模板 ID**、单次调用超时；
5. 保存后去「站点 → 登录策略」打开「**邮箱验证码登录**」。

Endpoint 只有国际站账号才需要改（默认 `ses.tencentcloudapi.com`）。保存后**下一次发信就生效**，不用重启。

验证：退出登录，在读者站登录框切到邮箱登录，点「获取验证码」；收不到就按顺序查：模板变量对不对、发信地址验证过没有、地域填错没、后台日志里的邮件错误。验证码有效期与发码限流在「站点 → 登录策略」里调（默认 5 分钟有效、60 秒窗口）。

---

## 三、它由什么组成

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

## 四、请求是怎么流向的

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

## 五、环境准备

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

## 六、配置

配置分三层，判据只有一句：**改它会不会影响"进程怎么跑"**。

### 6.1 三层配置的分工

```
第一层  BLOGX_* 环境变量         ← 基础设施与进程级：库、缓存、JWT、监听地址、日志目录、CDC
            │
            ▼  占位符替换（${VAR}）
第二层  backend/config/settings.yaml            ← 进程级基线（不含业务配置）
        backend/config/site_default_settings.yaml ← 站点业务默认值（字面量，无环境变量）
            │
            ▼  首次启动写入数据库
第三层  数据库 runtime_config_models 表         ← 后台「站点」页八个 Tab 改的是这里，以此为准
```

关键点：

- **业务配置不在环境变量里**。邮件（腾讯云 SES）、AI、对象存储、QQ 登录、登录策略、日志策略、上传策略，一共 52 个 `BLOGX_*` 已删除，改成在**管理员后台**配置：容器拉起来 → 打开后台 → 第一次进后台时设置管理员密码 → 「站点」页八个 Tab 里填。改完即时生效，不用重启容器。
- **`settings.yaml` 里剩下的全是 `${BLOGX_*}` 占位符**，没设变量会直接 panic 并告诉你缺哪几个——这是特性：宁可起不来，也别带着半套配置跑。剩下的是自举必需项（没有 DB 地址就连不上装配置的那张表）。
- **`site_default_settings.yaml` 全是字面量**，只在数据库还没有该配置段时作为初始值写入。之后以数据库为准；想回到默认值，清掉对应的运行时配置记录（`runtime_config_models` 里按 `name` 删一行）。
- **`.env` / `.envrc` 都不进仓库**（`.gitignore` 已排除），模板是 `.envrc.example`（跑在宿主机上，带 `export` 前缀）和 `deploy/compose/production/env.example`（跑在容器里，无前缀）。两份键名一致，**唯一必须不同的一项是 `BLOGX_APP_HOST`**：宿主机用 `127.0.0.1`，容器内必须 `0.0.0.0`。改密码或 JWT 时两处要一起改。

设计说明与代价见 [`docs/architecture/configuration.md`](docs/architecture/configuration.md)；
AI 的模型配置（多条供应商 + 启用其一）是独立一张表，见
[`docs/architecture/ai-integration.md`](docs/architecture/ai-integration.md)。

### 6.2 后端必填变量

完整清单见 `deploy/compose/production/env.example`（约 50 项，含注释）。**真正必须自己填的只有下面这些**，其余可以照抄示例值：

| 变量 | 作用 | 不填的后果 |
| --- | --- | --- |
| `BLOGX_JWT_SECRET` | 登录态签名密钥 | 泄露 = 任何人都能伪造登录态。**必须换成高强度随机串** |
| `BLOGX_JWT_ISSUER` | 签发者标识 | 用示例值也能跑，但建议改 |
| `BLOGX_DB_PASSWORD` | MySQL 业务账号密码 | 示例是 `******` |
| `BLOGX_DB_REPL_USER` / `BLOGX_DB_REPL_PASSWORD` | 主从复制账号 | 从库连不上，读写分离失效 |
| `BLOGX_REDIS_PASSWORD` | Redis 密码 | 容器渲染配置时用到 |
| `BLOGX_ES_PASSWORD` | ES `elastic` 用户密码 | 健康检查、river 都会失败 |
| `BLOGX_CLICKHOUSE_PASSWORD` | ClickHouse 密码 | 日志写不进去（会落本地 WAL） |
| `BLOGX_APP_ENV` | `dev` / `test` / **`production`** | `production` 给 refresh Cookie 加 `Secure`；`prod` 是旧写法（仍兼容，启动日志会告警）；http 环境必须是 `dev` |
| `BLOGX_GIN_MODE` | `debug` / `release` | 生产用 `debug` 会打印大量 SQL 与调试信息 |
| `BLOGX_APP_HOST` | 监听地址 | **容器内必须是 `0.0.0.0`**（见下方陷阱） |
| `BLOGX_NGINX_ENV` | 部署形态开关，见 [11.2](#112-nginx-的四种形态) | 决定 nginx 是否要求域名与证书 |
| `BLOGX_NGINX_SERVER_NAME` | 覆盖 nginx `server_name`，留空用形态默认值 | 只影响日志与 `nginx -T` 里的域名；单 server 块下不影响访问 |

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

### 6.3 第三方服务（按需开启）

这一组**不再用环境变量**，全部在管理员后台「站点」页配置。不配也能跑，只是对应功能不可用：

| 后台 Tab | 功能 | 关闭 / 未配时 |
| --- | --- | --- |
| 邮件（腾讯云 SES） | 邮箱验证码登录、注册、绑定邮箱 | 「站点 → 登录策略」里的邮箱登录默认是**关的**；没配 SES 就打开，用户点「获取验证码」会直接报错 |
| 第三方登录（QQ 互联） | QQ 登录 | 读者站目前没有 QQ 登录入口，配好也不生效（链路待补） |
| 对象存储（七牛云） | 图片上传（文章配图） | 上传接口返回「站点未启用图片上传」。**没有本地上传兜底**：`/uploads` 只是静态路由，代码里没有写入它的逻辑。接入步骤与证书自动化见[第十六章](#十六七牛云接入与证书自动化) |
| AI 助手 | 站内 AI 助手（搜索/润色/元信息/评分） | 默认关闭。这里有两层：**总闸**（启用 AI 助手）+ **模型配置列表**里启用其中一条，两者都满足才可用 |
| AI 助手 → 模型配置 | 供应商与密钥 | 支持 OpenAI / Claude / Gemini / 火山方舟 / DeepSeek / 通义千问 / 百度千帆 / OpenRouter，可并存多条、启用一条。没有启用的配置时调用会报「还没有启用的模型配置」 |
| 站点 → 登录策略 | 登录方式、验证码、限流与失败锁定 | 只留用户名密码登录也能正常用 |
| 日志 | 日志级别、请求日志、查询限额、清理窗口 | 级别即时生效；日志**目录**与输出格式是进程级的，UI 上只读 |
| 上传策略 | 允许上传的图片格式白名单 | 白名单是整体替换，删一项保存后该格式立刻不能传 |

邮件用的是腾讯云 SES 的 `SendEmail`，**只支持模板发信**：正文由控制台审核过的模板渲染，
后台只填模板 ID。四个场景（注册 / 重置密码 / 绑定邮箱 / 邮箱登录）共用同一模板，
变量是 `action` / `code` / `timeout`。地域只支持 `ap-guangzhou` / `ap-hongkong`。

AI 的 Base URL 必须是 **Chat Completions 风格**的完整地址（例如 `https://api.openai.com/v1/chat/completions`），不是 base 域名——填错表现为 404 而不是参数错误。

还有两个 CDC（MySQL → ES / 图片引用）**仍然由环境变量控制**，因为它们决定要不要起一个 binlog 消费者，属于"进程怎么跑"：

| 变量 | 作用 | 关闭后 |
| --- | --- | --- |
| `BLOGX_RIVER_ENABLED` | MySQL → ES 同步 | 搜索功能不可用（索引不会更新） |
| `BLOGX_IMAGE_REF_RIVER_ENABLED` | 图片引用关系同步 | 图片引用统计不准 |

### 6.4 前端配置：profile 机制

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

## 七、本地开发

有两种玩法，**推荐第二种**：基础设施用 Docker，应用在宿主机跑。改代码能热更新，断点也能打。

### 7.1 第一步：起基础设施

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

### 7.2 第二步：注入后端环境变量

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

### 7.3 第三步：初始化数据库与索引

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

### 7.4 第四步：设置管理员密码

**没有默认账号**。第一次打开后台（`http://localhost:3001/admin/`）会被跳到 `/setup`，
在那里设用户名和密码——这是现在的主路径，不需要记任何命令。

这个入口只在**库里一个管理员都没有**时开放：判断只看角色不看状态（一个被禁用的管理员
也算"已经初始化过"，否则停用账号会让入口重新对所有人开放）。调用时角色被强制写成管理员，
请求体里的 role 一律忽略。

想用命令行建号也可以（适合自动化部署）：

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

### 7.5 第五步：启动两个前端

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

### 7.6 访问地址

| 地址 | 是什么 |
| --- | --- |
| http://localhost:3000 | 读者站 |
| http://localhost:3001/admin/ | 管理后台 |
| http://127.0.0.1:8080/health | 后端健康检查 |
| http://127.0.0.1:8080/api/... | API 直连 |
| http://127.0.0.1:9200 | ES（`elastic` / `.envrc` 里的密码） |

注意后台的**尾斜杠**：`/admin` 在 nginx 里是 301 跳到 `/admin/`，本地直连 3001 时也要带 `/admin/`，否则 404。

### 7.7 只想全部跑在 Docker 里？

```bash
cd deploy/compose/production     # 生产编排同样能本地跑
cp env.example .env              # 然后把 BLOGX_NGINX_ENV 改成 local
docker compose up -d --build
# 浏览器打开 http://localhost
```

`BLOGX_NGINX_ENV=local` 时 nginx 不要求证书、只监听 80，两个子站都走容器内 upstream。想省内存就把 `BLOGX_NGINX_ENV` 改成 `api-only`，只留 API。

---

## 八、前后端契约（唯一耦合点）

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

## 九、后端命令行参考

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

## 十、门禁（提交前必跑）

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

## 十一、部署到服务器

### 11.1 前置条件

| 项 | 建议 |
| --- | --- |
| 系统 | Linux（x86_64），Docker 24+ 与 Compose v2 |
| 内存 | **8 GB 起步，16 GB 更稳**。compose 里各服务 limits 合计约 9.8 GB（那是上限不是占用） |
| 磁盘 | 40 GB+（MySQL 数据 + ES 索引 + ClickHouse 日志 + 上传图片） |
| 端口 | 80 **和** 443，其余一律不开（见下面那条最容易漏的） |
| 域名 | 已解析到服务器 IP，证书已备好 |

> **80 和 443 是两条独立的安全组规则，要分别放行。**
> 云平台（腾讯云 / 阿里云 / 轻量应用服务器各自叫安全组或防火墙）里放行 80 **不等于**放行 443。
> 只放 80 的症状很具体：`http://域名` 正常、`https://域名` 死活打不开，浏览器通常是「连接超时」而不是证书错误。
> 这种情况下服务器侧一切正常（`curl -kI https://127.0.0.1/` 返回 200），**包在到达服务器之前就被丢了**，
> 排查路径见[常见问题](#十四常见问题)里「HTTP 能打开、HTTPS 打不开」。

Linux 上 ES 还需要（否则容器反复重启，日志里是 `max virtual memory areas vm.max_map_count [65530] is too low`）：

```bash
sudo sysctl -w vm.max_map_count=262144
echo 'vm.max_map_count=262144' | sudo tee -a /etc/sysctl.conf
```

### 11.2 nginx 的四种形态

部署形态由**一个变量** `BLOGX_NGINX_ENV` 决定，`render-config.sh` 据此推导域名、证书、upstream 与子站开关：

| 形态 | 域名 | 443 + 证书 | `/` | `/admin/` |
| --- | --- | --- | --- | --- |
| `local` | `localhost` | 否（只监听 80） | 读者站 | 后台 |
| `test` | `blog.gentlybeing.cn` | 是 | 读者站 | 后台 |
| `production` | `blogx.gentlybeing.cn` | 是 | 读者站 | 后台 |
| `api-only` | `blog.gentlybeing.cn` | 是 | 404 | 404 |

要换域名，在 `.env` 里加 `BLOGX_NGINX_SERVER_NAME='你的域名'`（compose 已透传，留空回退默认值）。
改 `render-config.sh` 里的默认值也能生效，但那是仓库文件，`git pull` 会和上游冲突，不推荐。

### 11.3 首次部署

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
BLOGX_APP_ENV='production'              # 走 https 才写 production（开 Secure Cookie）；http 环境写 dev
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

> 不想手动准备证书：用 acme.sh 签发并自动续期（续期后自动重载 nginx），命令见[快速开始 · 场景 2](#2-服务器-https-docker-拉起)的 B 方案。

首次启动会自动初始化，不用手动跑迁移。`deploy/docker/api/entrypoint.sh` 会先查 `runtime_site_config_models` 表：

- **表不存在** → 自动执行建表，等 ES 就绪，再 `-es -s ensure`，然后才真正启动服务；
- **表已存在** → 跳过初始化，直接启动。

这带来一个副作用要知道：**想重新初始化，得先把这张表删掉**（或手动执行 `-db`），否则 entrypoint 永远认为"已经初始化过了"。

**启动要多久**：首次 `up` 含构建，几分钟；之后重启约 **40 秒**，其中大头是 ES 自己的冷启动（entrypoint 的 `wait_for_es` 每 3 秒探一次），应用连上 Redis/MySQL/ES 本身只要几毫秒。`blogx_server` 的探针间隔是 10s，所以"进程就绪"到"被判定 healthy"最多再差 10 秒。

```bash
# 5. 确认状态
docker compose ps                       # 全部 Up (healthy)
docker compose logs -f blogx_server     # 看启动日志

# 6. 创建管理员（同本地步骤）
docker compose exec blogx_server /app/server \
  -f config/settings.yaml -t user -s create \
  -user-role admin -user-username admin -user-password '<强密码>'

# 7. 冒烟测试
curl -sI https://你的域名/health    | head -1
curl -sI https://你的域名/          | head -1
curl -sI https://你的域名/admin/    | head -1
curl -s  https://你的域名/api/site/seo | head -c 200; echo
```

顺带查一下集群状态，正常应是 `green`：

```bash
docker compose exec es sh -c 'curl -s -u elastic:$ELASTIC_PASSWORD \
  "http://127.0.0.1:9200/_cluster/health?pretty"' | head -6
```

单节点也是 `green` 才对：索引定义里用的是 `auto_expand_replicas: "0-1"`（按数据节点数自动取值，1 节点 → 0 副本），所以不会再出现"永远 yellow"。

另外 `entrypoint.sh` 会在 `blogx_server` 里拉起一个 ES 看门狗（`deploy/docker/api/es-watch.sh`），每 60 秒采样一次、**只在状态变化时**写日志，前缀 `[es-watch]`。它记录的正是 `docker compose ps` 看不见的东西——ES 中途挂过、重启过。用法见[常见问题](#十四常见问题)。

`api/site/seo` 那条最有价值：它同时穿过 nginx 反代、Go 服务、MySQL 连接三件事，而前三条任何一条挂了都可能只是缓存或纯前端的问题。

**但上面这四条在服务器上跑，只能证明"nginx 和后端活着"，证明不了"公网能进来"** —— 它们走的是 127.0.0.1，根本不经过安全组。真正的上线判定必须从外部做一次：

- 手机流量（关掉 WiFi）打开 `https://你的域名`，或
- 从任意另一台机器访问，或
- 用外部探测（SSL Labs、`curl` 一台别的服务器）

这一步不过，基本就是安全组没放行 443，跟证书、nginx、后端都无关。

浏览器访问 `https://你的域名`（读者站）与 `https://你的域名/admin/`（后台）。

### 11.4 更新版本

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

### 11.5 回滚

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

### 11.6 上线检查清单

- [ ] `BLOGX_APP_HOST=0.0.0.0`、`BLOGX_NGINX_ENV=production`、`BLOGX_GIN_MODE=release`
- [ ] 五个密码全部换成强密码，`BLOGX_JWT_SECRET` 已重新生成
- [ ] 证书放在 `deploy/state/nginx/cert/domain.pem` / `domain.key`，私钥权限 600
- [ ] 安全组/防火墙**分别**放行了 80 与 443；`docker compose ps` 里 mysql/redis/es/clickhouse **没有**宿主机端口映射
- [ ] `curl https://域名/health` 返回 200（服务器上跑，只证明进程活着）
- [ ] **从外网访问一次 `https://域名` 成功**（手机流量 / 另一台机器）。服务器上 curl 127.0.0.1 永远通，它不过安全组，证明不了公网可达
- [ ] ES 集群 `"status": "green"`（单节点靠 `auto_expand_replicas: "0-1"`，不该是 yellow）
- [ ] `/` 与 `/admin/` 都能打开，后台能登录（已用命令行建号）
- [ ] 上传一张图片，确认返回七牛 CDN 域名且能打开。**没配七牛时这一项会报「站点未启用图片上传」，属预期**
- [ ] 发一篇文章，等几秒后在站内搜索能搜到（验证 river 链路）
- [ ] 后台日志页有数据（验证 ClickHouse 链路）
- [ ] 备份任务已配置（见 [12.3](#123-备份与恢复)）

---

## 十二、运维手册

### 12.1 端口与容器

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

`nginx` 那行的 80 / 443 是**容器侧**的映射，宿主机外面还有一层云平台的安全组/防火墙，两边都要放开才算通。只开 80 的典型表现见[常见问题](#十四常见问题)「HTTP 能打开、HTTPS 打不开」。

### 12.2 常见运维命令

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

# 集群状态：正常应是 green；yellow 说明有副本分不出去（见常见问题）
docker compose exec es sh -c 'curl -s -u elastic:$ELASTIC_PASSWORD \
  "http://127.0.0.1:9200/_cluster/health?pretty"' | head -6

# 索引是不是"正规建出来的"：数一下 IK 分词器出现次数，0 = 裸索引（搜索会不准）
docker compose exec es sh -c 'curl -s -u elastic:$ELASTIC_PASSWORD \
  "http://127.0.0.1:9200/article_index/_mapping"' | grep -c ik_max_word

# ES 有没有挂过 / 重启过（由容器内的看门狗记录，只在状态变化时写）
# 用 -F 按纯字符串匹配：方括号和反斜杠都不用转义，复制粘贴不会走样
docker compose logs blogx_server | grep -F 'es-watch'

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

### 12.3 备份与恢复

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

### 12.4 日志去哪看

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

## 十三、代码约定（改代码前请读）

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

## 十四、常见问题

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
- `max virtual memory areas vm.max_map_count [...] is too low` —— Linux 上内核参数太小，见 [11.1](#111-前置条件)。
- OOM / `Killed` —— 内存上限小于 512 MB。`ES_JAVA_OPTS` 已经是 `-Xms512m -Xmx512m`，上限再小就会 OOM。

**怎么知道 ES 中途挂过 / 重启过**
`docker compose ps` 看不出来：`restart: always` 会把它自己拉回来，重启完照样 healthy。
`entrypoint.sh` 会在 `blogx_server` 里拉起一个看门狗（`deploy/docker/api/es-watch.sh`），
每 60 秒采一次 `_cluster/health` 和 JVM uptime，**只在状态变化时**写一行，前缀 `[es-watch]`：

```bash
docker compose logs blogx_server | grep -F 'es-watch'
```

会看到 `检测到 ES 重启：JVM uptime ... -> ...`、`集群状态变化 green -> yellow`、
`ES 不可达` 这类行。uptime 倒退是唯一可靠的“重启过”信号——即使重启正好发生在两次采样之间也能发现。
它只记录、不干预；要主动通知得另接外部监控。

**HTTP 能打开、HTTPS 打不开**
最典型的原因是**云平台只放行了 80、没放行 443**，不是证书也不是 nginx 的问题。按这个顺序定位，三步就能定性：

```bash
# ① 服务器上直连 443：通，说明 nginx + 证书 + 端口映射全都是对的
curl -sI https://127.0.0.1/ | head -1
curl -I --resolve 你的域名:443:127.0.0.1 https://你的域名/    # 不加 -k，顺便验证证书与域名匹配

# ② 抓包看外部请求的 SYN 到底有没有到网卡（跑起来后用浏览器访问一次）
sudo timeout 40 tcpdump -ni any 'tcp port 443 and tcp[tcpflags] & tcp-syn != 0'

# ③ nginx 有没有收到握手痕迹
docker compose exec nginx tail -20 /var/log/nginx/error.log
```

**②③ 都是空的时候（`0 packets captured`、error.log 没有新内容）= 包在到达服务器之前就被丢了**，那只可能是安全组/防火墙。去控制台放行 `TCP:443`，几秒生效，不用重启任何服务。

两个容易把自己带进坑里的点：

- **别用本机的 `Test-NetConnection` / `curl` 判断公网可达性**。开着 Clash 之类的代理（TUN 模式 + fake-ip）时，域名会解析成 `198.18.x.x`，你测到的是本机代理的监听口，返回 `TcpTestSucceeded: True` 也说明不了任何事。要么用手机流量（关 WiFi），要么在服务器上抓包。
- 浏览器报错要分清：`连接超时` = 包被丢（防火墙）；`ERR_SSL_PROTOCOL_ERROR` / 证书告警 = 通了但握手或证书有问题（先确认访问的不是 `www.` 前缀，证书 SAN 里可能只有裸域名）。

**集群一直是 yellow / `unassigned_shards` 大于 0**
单节点集群里索引默认 1 副本，那份副本永远分配不出去，状态就卡在 yellow。项目里的索引定义用的是 `auto_expand_replicas: "0-1"`（1 个数据节点 → 0 副本 → green；多节点 → 1 副本 → 有冗余），所以**新建的索引不会再 yellow**。

已存在的索引要单独改，不重建、不丢数据：

```bash
docker compose exec es sh -c 'curl -XPUT -u elastic:$ELASTIC_PASSWORD \
  -H "Content-Type: application/json" \
  "http://127.0.0.1:9200/article_index/_settings" \
  -d "{\"index\":{\"auto_expand_replicas\":\"0-1\"}}"'
```

yellow 本身不影响读写，但它会让"颜色"这个最直观的信号失效——将来真出问题（主分片丢失是 red、副本缺失是 yellow）时你分辨不出来。别把它当常态放着。

**搜索搜不到文章，但数据库里明明有**
按顺序查：`BLOGX_RIVER_ENABLED` 是否为 `true` → river 日志有没有报 binlog 权限 → 索引是否存在 → 手动回填一次（`-es -s article-sync`）。注意写库后在 ES 里能搜到**有几秒延迟是正常的**。

还有一种更隐蔽的可能：**索引被删过，ES 自己建了个裸索引补上**。判断方法：

```bash
docker compose exec es sh -c 'curl -s -u elastic:$ELASTIC_PASSWORD \
  "http://127.0.0.1:9200/article_index/_mapping"' | grep -c ik_max_word
```

返回 `0` 就是裸索引——没有 IK 分词器（中文按单字切）、没有 `index.default_pipeline`、字段类型是动态推断的。表现是"能搜到但搜不准"或者干脆搜不到，而 `/health` 照样 200、容器照样 healthy。

之所以会这样：`-es -s ensure` 只在**首次初始化那天**跑过一次（`entrypoint.sh` 用 `runtime_site_config_models` 表是否存在判断），之后启动都不再建索引，服务运行期也没有任何索引存在性检查。这时要手工修：

```bash
docker compose exec blogx_server /app/server -f /app/config/settings.yaml -es -s delete   # 删掉裸索引
docker compose exec blogx_server /app/server -f /app/config/settings.yaml -es -s ensure   # 按 mapping 重建
docker compose exec blogx_server /app/server -f /app/config/settings.yaml -es -s article-sync   # 全量回填
```

`delete` 是破坏性的，只删 ES 里的索引，**不动 MySQL 数据**，所以 `article-sync` 能原样灌回来。

**后台登录报 401 / 打开 `/admin/` 是 404**
401：账号没建——第一次打开后台会自动跳到 `/setup` 设密码；想用命令行就 `-t user -s create`。404：少了尾斜杠，或者 `BLOGX_ADMIN_BASE_URL` 与 nginx 的 `location /admin/` 对不上。

**上传图片报「站点未启用图片上传」**
不是 bug，是没配七牛。图片上传只有七牛这一条落地路径（后台「站点 → 对象存储」页：开关 + AK/SK + bucket + 访问域名），
**没有"存本地"的兜底**——`/uploads` 只是静态路由，代码里没有任何写入它的逻辑。要用文章配图就先去七牛建空间。
从建空间到验证连通的完整步骤（含证书自动上传与续期）见[第十六章](#十六七牛云接入与证书自动化)。

**图片上传成功但页面显示不出来**
先看接口返回的 URL 是不是 CDN 域名（后台「对象存储」里的访问域名，**要带协议头**：`https://` 或 `http://`；缺了会直接让上传任务失败，而不是显示问题）。浏览器侧只经 nginx 的
`/uploads/` 与后端静态路由走本地文件，CDN 图片是直连的，跟容器无关——所以"容器里看不到图"是正常的。
前端还有 `BLOGX_WEB_ASSET_PROXY_BASE`（默认 `/uploads`）这个前缀，改错会让所有本地图片 404。
直链 **401** 是空间私有读；HTTPS 页面加载 HTTP 图片会被当 mixed content 拦掉——两类都见[16.7 常见故障对照](#167-常见故障对照)。

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

## 十五、需要时可查的文件

README 只能回答"怎么用"。更细的东西都在代码库里，而且它们是**现行事实**——改了代码它们跟着变，不会过期：

| 想知道 | 看哪里 |
| --- | --- |
| 全部环境变量、默认值、哪些必须改 | `deploy/compose/production/env.example`（带注释的完整清单） |
| 后端每个配置项什么意思 | `backend/config/settings.yaml`（逐字段中文注释） |
| 站点标题 / 备案号 / 登录开关的默认值 | `backend/config/site_default_settings.yaml` |
| 为什么某些配置在后台改、某些必须在环境变量里 | `docs/architecture/configuration.md` |
| AI 用哪家模型、怎么再加一个供应商 | `docs/architecture/ai-integration.md` |
| 表结构 | `backend/internal/platform/mysql/ddl/schema.sql` |
| 接口清单与出入参结构 | `backend/docs/swagger.json`，或前端 `app/types/openapi.d.ts` |
| nginx 的路由与反代规则 | `deploy/docker/nginx/nginx.conf.tmpl`；变量推导在 `render-config.sh` |
| 容器启动前做了什么（初始化、等 ES、拉起看门狗） | `deploy/docker/api/entrypoint.sh` |
| ES 状态变化的判定逻辑（uptime 倒退 = 重启过） | `deploy/docker/api/es-watch.sh` 顶部注释 |
| ES 索引的 mapping 与那两条"改错不报错"的约定 | `backend/internal/platform/es/articlesync/mapping/article_mapping.json` + `rows.go` 里的 embed 注释 |
| 容器编排、依赖顺序、资源上限 | `deploy/compose/production/docker-compose.yml` |
| 门禁到底检查了什么 | `scripts/check.sh` 顶部注释（每个检查项都写了为什么） |
| 架构守卫规则与当前基线 | `backend/tools/archcheck/`、`scripts/arch-baseline.env` |
| CI 有哪些 job、各自拦什么 | `.github/workflows/ci.yml` |

改代码前最该看的一处不是文档，是 `schema.sql` 和 `archcheck` 的规则——前者告诉你哪些字段不能动，后者告诉你哪些写法会被门禁拦下。

---

## 十六、七牛云接入与证书自动化

图片上传**只有七牛一条落地路径**：`/uploads` 只是静态路由，代码里没有任何写入它的逻辑。所以"配好七牛"不是可选项，而是用文章配图的前提。这一章分两块：**16.1–16.3 接入平台**，**16.4–16.7 证书自动上传与自动续期**（七牛只给 API，不给续期，链路要自己编排）。

### 16.1 七牛控制台侧（一次做完）

| 步骤 | 做什么 | 容易错的地方 |
| --- | --- | --- |
| 建空间 | 对象存储 → 新建空间，记下空间名与**区域** | 区域要对应平台里的「存储区域」；错区上传会失败（`z2` = 华南） |
| 访问控制 | 空间设**公开读**（或自己给直链加签名） | 平台返回的图片直链是**未签名 URL**；私有空间 → 浏览器 401 |
| 密钥 | 个人中心 → 密钥管理 → AK / SK | SK 只填在后台，别写进仓库 |
| 绑定加速域名 | 域名管理 → 添加域名（如 `image.example.com`）→ 复制 CNAME → 去 DNS 服务商加 CNAME 记录 → 回七牛点「验证」 | 只加解析不点验证 = 域名不生效；国内加速要求域名已备案 |
| 协议 | 先只绑 HTTP 也能用，但**平台回调必须是 HTTPS** | 后台「访问域名」的协议头必须与七牛实际支持的协议一致，见 16.2 / 16.4 |

七牛的 CDN 配置改动一般 **5–10 分钟**生效，别改完立刻判死。

### 16.2 平台侧（后台「站点 → 对象存储」）

| 字段 | 填什么 |
| --- | --- |
| 启用七牛云存储 | 开 |
| AccessKey / SecretKey | 七牛 AK / SK。SK 保存后显示 `******`，**留 `******` = 不修改** |
| Bucket | 空间名（如 `myblogx`） |
| **访问域名** | 写 `https://image.example.com`（推荐）。后端会把裸域名 / `http://` / `//` 统一收敛成 `https://`（见 `qiniu.go` 的 `mediaBase`）；但**已经入库的旧 `http://` URL 不会自己变**，要按 16.7 单独批量修正。另外 CDN 域名必须真的配了 HTTPS 证书 |
| 存储区域 | 与空间区域一致 |
| 对象 key 前缀 | 默认 `myblogx`。image-ref 靠它从图片 URL 反解 object key，**改了已有图片的引用关系就对不上** |
| 单图上限 / 凭证有效期 | 默认 5MB / 1800s。有效期是**上传凭证**的寿命，慢网大图容易过期（七牛回 401） |
| 上传回调地址 | `https://<主域名>/api/images/qiniu/callback`。**必须 HTTPS 且证书覆盖该主机名**；**留空 = 不启用回调**，任务永远不会被确认（一直 pending，前端最后报"上传确认超时"） |

保存即热生效（运行时配置现读，不用重启）。

### 16.3 验收到什么程度算接通

1. 后台上传一张图（媒体库或轮播图）。
2. 服务器上看回调有没有到：
   ```bash
   grep 'qiniu/callback' /opt/blogx/deploy/state/nginx/logs/access.log | tail -3
   ```
   应出现**七牛节点 IP** 的 `POST /api/images/qiniu/callback`，状态 200。
3. 数据库应该有行（图片记录 + 任务 ready）：
   ```bash
   docker exec mysql-master mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -N \
     -e "select count(*) from blogx.image_models; select count(*) from blogx.banner_models"
   ```
4. 拿返回的图片 URL 测直链：
   ```bash
   curl -sS -o /dev/null -w '%{http_code}\n' "https://image.example.com/<key>"
   ```
   200 才对；401 = 空间私有（回 16.1 改访问控制）。
5. 后台是 HTTPS、图片域名还是 HTTP 时，图片会被浏览器拦掉——**那是预览被拦，不是上传失败**。彻底解决走 16.4 给图片域名绑 HTTPS。

> **浏览器对 HTTPS 页面里的 HTTP 图片会"自动升级"成 HTTPS 再加载（Chrome / Firefox / Safari 都这样），这个行为没有开关，服务端也关不掉。** 所以只能让图片本身是 HTTPS；改配置、加 CSP 之类都没法"关闭"它。

### 16.4 先弄清两件事

- **七牛只有"上传 / 绑定"的 API，没有"续期"**。证书就是一段 PEM，续期要自己用 ACME 签发再推上去。
- **图片域名走不了 HTTP-01**：它 CNAME 到七牛 CDN，ACME 的验证文件落不到我们服务器 → 必须 **DNS-01**（去 DNS 服务商加 TXT 记录）。而主站 DNS 在火山引擎，所以要一个火山引擎 DNS 插件。

链路：

```
acme.sh（每天 cron，证书 <30 天自动续）
  └ DNS-01：调火山引擎 API 加/删 TXT 记录
      └ 续期成功 → --reloadcmd 触发 qiniu-cert-sync.sh
          ├ POST fusion.qiniuapi.com/sslcert          （QBox 鉴权）→ 拿 certID
          ├ PUT  api.qiniu.com/domain/<域名>/sslize   （Qiniu 鉴权）→ 绑定新证书
          └ 校验 https 可用；旧证书确认解绑后再删
```

> ⚠️ **两套证书别混**：主站 nginx 用 `deploy/state/nginx/cert/domain.pem`（裸域 + www，certbot 签发，见第十一/十二章），七牛 CDN 域名用的是**另一张**、只对这个 CDN 域名生效。主站证书**不能**拿去给七牛用（SAN 里没有 `image.*`）。

### 16.5 签发（acme.sh + 火山引擎 DNS 插件）

```bash
curl https://get.acme.sh | sh -s email=you@example.com
source ~/.bashrc

# 火山引擎没有内置 DNS 插件，用社区脚本
curl -sSfL https://raw.githubusercontent.com/mitkimi/acmesh_volcengine_api/main/dns_volcengine.sh \
  > ~/.acme.sh/dnsapi/dns_volcengine.sh
chmod +x ~/.acme.sh/dnsapi/dns_volcengine.sh

# 火山引擎「访问控制 → API 访问密钥」拿 AK/SK；首次签发后 acme.sh 会加密存进 account.conf
export VOLCENGINE_ACCESS_KEY_ID='<AK>'
export VOLCENGINE_SECRET_ACCESS_KEY='<SK>'

acme.sh --issue --dns dns_volcengine -d image.example.com --keylength 2048
```

### 16.6 推送到七牛（scripts/qiniu-cert-sync.sh）

两个 API 用**两套鉴权**，别混：

| API | 域名 | 鉴权头 | 签名串 |
| --- | --- | --- | --- |
| 证书管理（上传 / 列表 / 删除） | `fusion.qiniuapi.com` | `QBox <AK>:<tok>` | `<路径+query>\n` |
| 域名配置（`sslize` 绑证书） | `api.qiniu.com` | `Qiniu <AK>:<tok>` + `X-Qiniu-Date` | `METHOD 路径+query\nHost\nContent-Type\nX-Qiniu-Date\n\nBody` |

脚本在仓库里：`scripts/qiniu-cert-sync.sh`。它从环境变量读凭据（**不进 git**），依次调上表两个接口，最后 `curl -sSI` 校验一次：

| 变量 | 作用 |
| --- | --- |
| `QINIU_AK` / `QINIU_SK` | 七牛密钥 |
| `CDN_DOMAIN` | 要绑证书的 CDN 域名 |
| `KEY_FILE` / `CHAIN_FILE` | acme.sh 落盘的私钥 / 证书链路径 |
| `CERT_NAME` | 可选，七牛控制台里的证书名（默认用 `CDN_DOMAIN`） |

手动重跑一次：`set -a; . /etc/blogx/qiniu/env; bash /opt/blogx/scripts/qiniu-cert-sync.sh`。

挂到 acme.sh，让每次续期后自动推：

```bash
mkdir -p /etc/blogx/qiniu
cat > /etc/blogx/qiniu/env <<'EOF'
QINIU_AK=<AK>
QINIU_SK=<SK>
CDN_DOMAIN=image.example.com
CERT_NAME=image.example.com
KEY_FILE=/etc/blogx/qiniu/image.key
CHAIN_FILE=/etc/blogx/qiniu/image.pem
EOF
chmod 600 /etc/blogx/qiniu/env

acme.sh --install-cert -d image.example.com \
  --key-file       /etc/blogx/qiniu/image.key \
  --fullchain-file /etc/blogx/qiniu/image.pem \
  --reloadcmd      'set -a; . /etc/blogx/qiniu/env; bash /opt/blogx/scripts/qiniu-cert-sync.sh'
```

`acme.sh` 安装时已往 crontab 写了 `acme.sh --cron`，每晚检查、剩 <30 天才续期，续完自动执行 `--reloadcmd`。全程无人值守（首次签发要人在场点一次）。

不想自建也可以：控制台手动上传证书（一年一次）、或用社区工具 [CertBuddy](https://github.com/Jyonn/CertBuddy)（腾讯云申请 + 七牛部署）。

### 16.7 常见故障对照

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| 浏览器报 **HTTP 579** | 七牛回调业务服务器失败（TLS 证书不匹配 / 域名不可达 / 回调路径 4xx-5xx） | 先自测回调：`curl -sS -o /dev/null -w '%{http_code}\n' -X POST https://<回调域名>/api/images/qiniu/callback -d '{}'`，能回 200 且 body 是业务码就说明网络与 TLS 通了 |
| `unsupported protocol scheme ""` | 旧版本：访问域名缺协议头，后端直接把它拼进 URL | 当前版本已把裸域名 / `http://` / `//` 统一收敛成 `https://`（`qiniu.go` 的 `mediaBase`），不会再出这个错；还出现说明跑的是旧镜像 |
| 上传后任务一直 pending / 前端报确认超时 | `callback_url` 空或不可达 | 看 nginx 有没有七牛的回调 POST；没有就是回调地址/证书问题 |
| 图片直链 **401** | 空间是私有的，直链没签名 | 空间改公开读，或给直链加签名（平台目前没做签名） |
| 图片 URL 能打开但页面不显示 | HTTPS 页面加载 HTTP 图片（mixed content），或前端 `BLOGX_WEB_ASSET_PROXY_BASE` 配错 | 给 CDN 域名绑 HTTPS 并把「访问域名」改 `https://`。浏览器的 HTTP→HTTPS **自动升级无法关闭**，所以不能保留 http 直链 |
| 上传七牛返回 401 / 631 | 401 = 上传凭证无效或过期（有效期 < 上传耗时）；631 = 空间不存在（bucket 名或区域错） | 凭证有效期别低于 60s；bucket 名与「存储区域」和七牛空间对齐 |
| `sslize` 回 500235 / 400302 | 域名未绑证书 / 域名未开 HTTPS | 顺序必须是**先上传证书拿 certID，再 sslize** |
| 证书传上去但 https 不生效 | CDN 配置 5–10 分钟生效 | 等 10 分钟再验；`curl -sSI https://<CDN域名>/` 看证书 SAN |
