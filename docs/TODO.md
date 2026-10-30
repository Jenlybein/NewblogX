# 待办与想法池

> 迁移自旧的 `docs/TODO.txt`（2026-10-29）。这里是**尚未排期**的想法与待办，不是承诺：已决策的进 `docs/adr/`，已交付的进 `docs/architecture/` 或 git 历史。条目保留原始表述（含链接、数字与量级），只做分组。

## 架构与基础设施

- 调查：如何进一步预备分布式 / 微服务的调整，目前已改用雪花 ID。
- 写个微服务，定期同步数据库与七牛云中图片结果，清除未记载在数据库的图片。
- 考虑升级成 PostgreSQL，首先得了解什么是 PgSQL。
- 水平分表：<https://www.wolai.com/fengfeng/pXg4Q6X9giMznrZiQg63jG>
- 了解 ELK：<https://www.cnblogs.com/crazymakercircle/p/18561362>
  最新架构使用：应用日志 / 容器日志 → Fluent Bit 或 Vector（采集器，轻量、比 Beats 简单）→ Kafka（可选，高并发才需要）→ Vector（清洗 / 路由，简单场景可跳过）→ ClickHouse 集群（真正存日志、做检索、做统计）→ Grafana / ClickVisual（类 Kibana 体验）→ Grafana Alert 或平台自带告警 <https://cloud.tencent.com/developer/article/1807679>
- docker compose 里存在密码，怎么办？后续全面转向环境变量配置：<https://www.doubao.com/chat/38419183522472450>

## 缓存与性能

- 各类展示信息做一层缓存。
- 检查各模块有没有性能问题。
- List 优化一次 HasMore 的形式（各种 List 列表看看怎么搞）；查询过多的接口，不再走统一 ListQuery，走专用 SQL 模式。
- 缓存优化中间件：把一些接口的响应记录到 redis 里，10 秒之内走缓存：<https://video.fengfengzhidao.com/play/17/3599>
  - 考虑：要加主动不走缓存的选项；缓存如何主动过期；如何防止恶意请求导致大量同一缓存内容出现不同 key 进入缓存。
  - 复用：复用 `audit_body_middleware`，改名成 `capture_middleware`，挂载时只能选择是否拦截 request 的 body 和 header（选项变成 None / Head / Body / Both）。
  - 到了接口内真正需要获取 `capReqBody` / `capHeader` bool 来判断日志是否记录请求体 / 头；检测 `gin.Context` 里有没有 `capRespBody` / `capHeader` bool 来选择是否拦截响应体 / 头。
  - 修改时减少历史遗留的不必要代码；没必要解耦的代码不强行解耦。
  - 以下接口可以加入（需判断还有哪些可以加入）：`BannerList`、标签列表。

## 数据库与数据模型

- setting 把 jwt 过期时间改回来。
- ID 发放改为号段模式：先去数据库申请一段 ID（比如 100000 ~ 100999），然后这 1000 个 ID 在内存里自己发，发完再去数据库申请下一段。
- 检查 redis 有没有持久化，避免数据回退。
- User 表可以细化分表：
  - `UserConfModel`、`UserStatModel` 不变。
  - `UserAuthModel`：Password / Email / OpenID / TokenVersion / LastPasswordChangedAt。
  - `UserModel`：Username / Status / Role / RegisterSource（还可以考虑怎么进一步分化）。
  - `UserProfileModel`：IP / Addr / Nickname / Avatar / Abstract / gender / 签名。
- 新增 `UserStatModel`：ViewCount / FansCount / FollowCount。
  - FansCount / FollowCount：做同步事务更新冗余字段。
  - ViewCount 大概方向：
    - 统计逻辑：用户主页每 1 个独立访客访问 +1，同一人当日多次访问仅计 1 次，游客访问不统计。
    - 去重方案：Redis HSETNX 记录 {访客ID:被访者ID}（具体 key 细节设置你来补充），整组 key 每日 0 点过期。
    - 数据库：新增 `UserViewDailyModel`（Model、UserID、ViewerUserID、ViewDate），建立唯一索引。
    - DB 更新方式：SQL 事务完成两个操作（原子插入日访问记录 + 更新总 ViewCount），无需提前查询。
    - 执行流程：Redis 写入式判重 → 无重复则数据库 INSERT + UPDATE → DB 失败时回滚 Redis。
    - 口径：「HSETNX 抢占式判重 + DB 事务两条 SQL + DB 唯一索引兜底 + 事务失败后最佳努力 HDEL 回滚 Redis」。
  - 接口升级：`api/user_api/profile_api/user_base_info.go` 返回字段增加 ViewCount / FansCount / FollowCount / FavoritesVisibility / FollowVisibility / FansVisibility / HomeStyleID。
- 校准系统：新增 cron 任务，定时校准冗余数据。平时校准不需要每天重算整个历史，只需获取今日有新插入的数据，然后做增量更新即可。
- 项目的原子性问题需要解决：redis 先存，mysql 后事务存，然后再回来确定 redis 是否真的成功存。

## 登录、安全与限流

- 可选用更新的交互式验证码：<http://gocaptcha.wencodes.com/guide/>
  验证码是 DDoS 攻击的良好接口之一，因为无需登录即可从服务器获取高成本响应。目前的防 DDoS 策略：登录失败 2 次后才出现验证码（同时发起 Cloudflare JS 质询）；每个 IP 每秒只能生成 1 次验证码；验证 10 次失败则拉黑 1 分钟。
- 并发安全：先查再建这种非原子操作，在并发环境下有概率导致重复建数据。
- 后续优化登录校验性能：
  1. 把「用户状态 + token_version + role + deleted_at」做短 TTL 缓存。
  2. 不要每次都查 `user_models`。
  3. 把 session 有效性做短 TTL 缓存，不要每次都查 `user_session_models`。
  4. 只把「撤销、改密、封禁」这些动作当成缓存失效触发器；如果访问量再上来，再考虑把黑名单和 session 状态合并成更紧凑的 Redis 校验链路。
- 完善 QQ 登录：QQ 登录 <https://video.fengfengzhidao.com/play/17/3391>
  发送手机验证码需要企业资质，而邮箱验证码可以用个人邮箱；因此目前项目的设置为必须绑定邮箱后才能依靠邮箱进行密码修改。
- 接口限制：每日数量限制，禁止重复操作过快。
- 创建接口测速功能。

## 搜索、AI 与内容

- 思考：把 tags、content_parts、content_head 改为用 river 传输。
- 站内知识问答：把已发布文章当知识库，做「基于文章内容回答问题」。比如用户问「这个站里有哪几篇讲 Go 中间件？」「帮我总结作者写过的 ES 相关文章」。这个比普通 AI 对话更有辨识度，也更能提升搜索体验。
- 语义搜索 / 混合搜索：现在明显已经有 ES 关键词搜索了，但还偏传统检索。可以升级成「关键词 + 向量召回 + rerank」，支持自然语言搜文，比如「有没有讲高并发缓存击穿处理的文章」，而不只是搜精确词。
- 评论与内容审核：文章发布审核、评论垃圾内容识别、辱骂 / 广告检测、低质量评论拦截、给管理员生成审核理由。这类功能与现有评论、文章审核流天然匹配，能直接降低运营成本。
- AI 博客助手：现有聊天系统可以复用，但建议做成「站内 AI 助手」而不是普通陪聊——回答站内内容问题、推荐文章、解释某篇文章重点、帮用户生成评论草稿、帮作者回复评论。
- 个性化推荐：结合浏览历史、收藏、点赞、标签偏好，做「你可能感兴趣的文章」「每周精选摘要」「新文章订阅推荐」。适合后面做，因为需要行为数据和推荐策略一起迭代。
- 多语言能力：自动生成英文摘要、中英双语文章、标题翻译、术语解释。对技术博客挺实用，尤其想做搜索流量扩展时。
- 推荐机制基础设想：文章推荐 = 当天浏览量最多的。
- AI 写作与评分（选中文本的即时操作，走 SSE 流式输出）：
  - 注意：固定 prompt 严格锁定输出边界，要求输出 JSON 格式并正则解析；正则解析工具放入 `ai_service/ai_utils.go`，所有 ai_service 通用。不要对很短的选区强行输出一堆建议，长度太短时直接提示「内容过短，建议选中完整句或完整段落」。
  - 内容改写：鼠标划线选中内容，弹出选项：内容润色、语法改正、风格转换；返回「替换后的文本」；润色最怕「把意思改跑了」，prompt 必须明确要求不新增原文没有的事实、不改变人名 / 时间 / 数字 / 结论、不乱补专业知识；只在表达、语法、结构、语气上优化，风格转换同样必须保留原始信息和观点。
  - 问题诊断：专门分析选中文本哪里有问题（专业性过强不易阅读、内容缺少、逻辑紊乱等），返回「问题类型 + 严重度 + 说明 + 原文证据 + 可执行建议」的结构化结果。示例：问题类型=可读性，严重度=中，说明=句子过长且术语密集，证据=第 2 句连续包含 3 个专业术语，建议=拆成两句并补一个通俗解释。
  - 上下文：传入时还要传一点上下文，前后各 300 到 800 字足够，不然成本会上去。请求字段：`mode`、`selection_text`（上限 3500 字 / 下限 25 字 / 必选）、`refix_text`（上限 800 字 / 可选）、`suffix_text`（上限 800 字 / 可选）、`article_title`（上限 50 字 / 必选）、`target_style`（风格转换时启用 / 可选）；上下限放 const。
  - 全文质量评分（同步完整内容输出）：拆成两个能力——「总体质量评分」（总分、维度分、主要问题、优先修改项）与「结构化写作建议」（标题不够聚焦、开头没有建立问题、论证顺序混乱、结尾缺少收束）。可以合并成一个接口返回，但内部 prompt 最好按统一 JSON 结构产出，否则前端很难稳定展示。至少 6 个维度：清晰度、结构性、完整度、可读性、说服力、语言规范。长文总分带权重（结构性 / 清晰度权重高一些，语言规范稍低；教程 / 科普提高完整度）；分数区间保守：常规文章 60–80，优质 81–88，精品 89–95，杜绝 96 分以上虚高。评分 prompt 写死标准、给模型明确 rubric；重复 / 相似问题去重归并，只保留最重要的前 0~6 个。返回结构：总分、维度分 `[{分,理由}]`、主要问题 `[{原文位置[{第几段,引用原文10~25个字}],问题说明,修改建议}]`（前 0~6 个）、总评价与建议。
  - 长文处理：15w 字以内直接全文评分。超长文按段切分（每段 ≤14w 字），每次传入都带完整保留标题、各级小标题；第一段单独进行一次完整 [评分 + 该段概括 + 完整]，然后把 [评分 + 该段摘要 + 问题] + 下一段交给大模型；第二段之后将 [评分 + 之前概括 + 新该段文章概括] 交给大模型，生成 [评分 + 该段概括 + 问题]；最后一段不再生成概括，输出最终结果。补充：[评分 + 该段摘要] 在同一次输出里获取，避免两次访问大模型加成本；prompt 可采取拼接方式（第一次、第二次到倒数第二次、最后一次分别拼接同一个核心评分 prompt）；重复问题去重归并。
  - 包结构建议：`ai_writing` 拆成 `ai_writing_types.go`（请求响应结构、枚举）、`ai_writing_rewrite.go`（内容润色 / 内容改正 / 风格转换）、`ai_writing_diagnose.go`（选中片段问题分析）、`ai_writing_prompt.go`（统一放 prompt 模板）；`ai_scoring` 拆成 `ai_scoring.go`（整篇评分与建议）、`ai_scoring_prompt.go`（统一放 prompt 模板）。
  - 前端交互：选中文本弹出的菜单要非常轻，最多三四个动作，不要做成二级菜单迷宫；点击后先弹一个小 loading，再展示「替换内容 / 追加建议 / 保留原文」三种操作；诊断类结果不建议自动替换原文，旁边列出问题卡片，点卡片时高亮原句；整篇评分像右侧面板，展示「总分 + 维度雷达图 / 条形图 + 主要问题 + 一键跳转定位到段落」。

## 工程与重构

- 需模块化的内容：`es_river`、`image_ref_river`、`cron_service`。
- 站点配置修改的接口是否已经完善？
- [P2] 写路径的副作用边界还不稳定，DB、缓存、消息仍然容易分叉。评论创建是典型例子：先写评论，再改审核状态，再起 goroutine 写站内消息，再回写 Redis 计数（`comment_create.go` line 72、line 85）；消息服务本身又直接用全局 DB 写表（`article_message.go` line 12）；文章删除虽然已经从 hook 迁出来了，但仍在事务体内直接改 Redis（`delete.go` line 18、line 84）。这类「主事务 + 最佳努力副作用」以后很适合收成显式领域服务 + post-commit / outbox。
- [P2] 聊天 WebSocket 入口还是一个过大的「超级处理器」，而且同源校验是全放开的（`CheckOrigin` 现在直接 `return true`，`chat_ws.go` line 32）；`ChatWsView` 同时负责鉴权回退、票据消费、连接生命周期、收件人校验、限流占位、消息落库、在线路由和错误回写（`chat_ws.go` line 63）。聊天功能一旦继续长大，测试、扩展 ack/retry、引入审核 / 风控都会越来越难。
