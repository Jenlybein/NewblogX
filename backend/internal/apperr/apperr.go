// Package apperr 是应用错误码。
//
// 为什么需要它：改造前全项目有 125 处 `errors.New("中文文案")`，前端只能匹配
// 中文字符串来决定分支，改一个字就断。archcheck R8 一直在拦这件事，但目标包
// 从未落地。这里补上，规则才有"正确的写法"可循。
//
// 设计约束：
//   - 只依赖标准库，因此 domain 层可以放心使用（不引入 gorm/gin）。
//   - 调用点只写错误码，中文文案集中在 messages 表里——
//     文案只有一处，将来做多语言或生成错误码文档都靠它。
//   - Error() 返回给人看的文案；判断分支一律用 CodeOf / IsCode，不要比字符串。
package apperr

import "errors"

// Code 是稳定的错误码。
//
// 命名用"领域.语义"：前端与日志都按它分支，一旦发布不可随意改。
type Code string

// 文章主体。
const (
	CodeArticleNotFound        Code = "article.not_found"
	CodeArticleForbidden       Code = "article.forbidden"
	CodeArticleInvalidState    Code = "article.invalid_state"
	CodeArticleAlreadyPublic   Code = "article.already_published"
	CodeArticleTitleRequired   Code = "article.title_required"
	CodeArticleContentRequired Code = "article.content_required"
	CodeArticleVersionConflict Code = "article.version_conflict"
)

// 分类与标签。
const (
	CodeTaxonomyTitleRequired Code = "taxonomy.title_required"
	CodeTaxonomyTitleExists   Code = "taxonomy.title_exists"
	CodeCategoryNotFound      Code = "taxonomy.category_not_found"
	CodeTagNotFound           Code = "taxonomy.tag_not_found"
	CodeTagDisabled           Code = "taxonomy.tag_disabled"
)

// 收藏。
const (
	CodeFavoriteTitleRequired Code = "favorite.title_required"
	CodeFavoriteFolderMissing Code = "favorite.folder_not_found"
	CodeFavoriteFolderExists  Code = "favorite.folder_exists"
	CodeFavoriteFolderDenied  Code = "favorite.folder_forbidden"
	CodeFavoriteExists        Code = "favorite.exists"
	CodeFavoriteNotFound      Code = "favorite.not_found"
	CodeFavoriteSelf          Code = "favorite.self"
)

// 置顶。
const (
	CodeTopLimitExceeded Code = "top.limit_exceeded"
)

// 审核任务。
const (
	CodeReviewTaskNotFound Code = "review.task_not_found"
	CodeReviewTaskHandled  Code = "review.task_handled"
	CodeReviewTaskDenied   Code = "review.task_forbidden"
)

// 用户资料与管理。
const (
	CodeUserNotFound         Code = "user.not_found"
	CodeUserForbidden        Code = "user.forbidden"
	CodeUserUsernameTaken    Code = "user.username_taken"
	CodeUserUsernameCooldown Code = "user.username_cooldown"
	CodeUserUsernameInvalid  Code = "user.username_invalid"
	CodeUserNicknameRequired Code = "user.nickname_required"
	CodeUserNicknameTooLong  Code = "user.nickname_too_long"
	CodeUserAbstractTooLong  Code = "user.abstract_too_long"
	CodeUserStatusInvalid    Code = "user.status_invalid"
	CodeUserStatusSelfLocked Code = "user.status_self_locked"
	CodeUserEmailTaken       Code = "user.email_taken"
	CodeUserEmailNotBound    Code = "user.email_not_bound"
	// CodeUserEmailRegistered / CodeUserEmailUnregistered 只在"发验证码"时用：
	// 邮箱的两种状态与四个发码场景不匹配时报错（注册/绑定要求未注册，
	// 登录/重置要求已注册），而不是静默不发信——见 docs/adr/0014。
	CodeUserEmailRegistered   Code = "user.email_registered"
	CodeUserEmailUnregistered Code = "user.email_unregistered"
	// CodeUserMailTooFrequent 是"发验证码太频繁"，与登录失败限流分开：
	// 共用 auth.too_frequent 时文案是"登录失败次数过多"，会把用户引到错误的方向。
	CodeUserMailTooFrequent   Code = "user.mail_too_frequent"
	CodeUserInvalidCredential Code = "user.invalid_credential"
	CodeUserAccountDisabled   Code = "user.account_disabled"
	CodeUserAccountBanned     Code = "user.account_banned"
	CodeUserPasswordRequired  Code = "user.password_required"
	CodeUserPasswordTooShort  Code = "user.password_too_short"
	CodeUserOldPasswordWrong  Code = "user.old_password_wrong"
	CodeUserPasswordSame      Code = "user.password_same"
	CodeUserSessionExpired    Code = "user.session_expired"
	CodeUserTokenInvalid      Code = "user.token_invalid"
	CodeUserCaptchaInvalid    Code = "user.captcha_invalid"
	CodeUserMailSendFailed    Code = "user.mail_send_failed"
	CodeUserOAuthFailed       Code = "user.oauth_failed"
	CodeUserRegisterDisabled  Code = "user.register_disabled"
)

// 认证与会话。
const (
	CodeAuthRequired       Code = "auth.required"
	CodeAuthTooFrequent    Code = "auth.too_frequent"
	CodeAuthRefreshInvalid Code = "auth.refresh_invalid"
	CodeAuthLoginDisabled  Code = "auth.login_disabled"
	// CodeAuthAdminRequired 表示"密码对了，但这个账号不是管理员"。
	// 只在管理后台的登录入口（scope=admin）返回，读者站不会出现。
	CodeAuthAdminRequired Code = "auth.admin_required"
)

// 评论。
const (
	CodeCommentNotFound       Code = "comment.not_found"
	CodeCommentClosed         Code = "comment.closed"
	CodeCommentForbidden      Code = "comment.forbidden"
	CodeCommentReplyInvalid   Code = "comment.reply_invalid"
	CodeCommentDiggChanged    Code = "comment.digg_changed"
	CodeCommentDiggDuplicated Code = "comment.digg_duplicated"
	CodeCommentContentEmpty   Code = "comment.content_empty"
)

// 图片与对象存储。
const (
	CodeImageUnavailable      Code = "image.unavailable"
	CodeImageUploadDisabled   Code = "image.upload_disabled"
	CodeImageTaskNotFound     Code = "image.task_not_found"
	CodeImageTaskFailed       Code = "image.task_failed"
	CodeImageTaskProcessing   Code = "image.task_processing"
	CodeImageInvalid          Code = "image.invalid"
	CodeImageTooLarge         Code = "image.too_large"
	CodeImageMismatch         Code = "image.mismatch"
	CodeImageAuditInvalid     Code = "image.audit_invalid"
	CodeImageDeleteFailed     Code = "image.delete_failed"
	CodeImageObjectKeyMissing Code = "image.object_key_missing"
	CodeImageCallbackInvalid  Code = "image.callback_invalid"
)

// 搜索。
const (
	CodeSearchTypeInvalid     Code = "search.type_invalid"
	CodeSearchSortInvalid     Code = "search.sort_invalid"
	CodeSearchPageModeInvalid Code = "search.page_mode_invalid"
	CodeSearchStatusRestrict  Code = "search.status_restricted"
	CodeSearchAuthorRequired  Code = "search.author_required"
	CodeSearchUnauthorized    Code = "search.unauthorized"
	CodeSearchForbidden       Code = "search.forbidden"
	CodeSearchResultInvalid   Code = "search.result_invalid"
)

// 关注关系。
const (
	CodeFollowSelf           Code = "follow.self"
	CodeFollowTargetNotFound Code = "follow.target_not_found"
	CodeFollowDuplicate      Code = "follow.duplicate"
	CodeFollowNotFollowing   Code = "follow.not_following"
	CodeFollowListPrivate    Code = "follow.list_private"
	CodeFollowConfigMissing  Code = "follow.config_missing"
)

// 私信。
const (
	CodeChatUserNotFound    Code = "chat.user_not_found"
	CodeChatContentEmpty    Code = "chat.content_empty"
	CodeChatTypeUnsupported Code = "chat.type_unsupported"
	CodeChatStrangerOff     Code = "chat.stranger_disabled"
	CodeChatNoPermission    Code = "chat.no_permission"
	CodeChatSessionNotFound Code = "chat.session_not_found"
	CodeChatEmptySelection  Code = "chat.empty_selection"
	CodeChatSendFailed      Code = "chat.send_failed"
	CodeChatTooFrequent     Code = "chat.too_frequent"
	CodeChatSessionTooFast  Code = "chat.session_too_fast"
	CodeChatWeekStranger    Code = "chat.week_stranger_limit"
	CodeChatWeekLimit       Code = "chat.week_limit"
)

// 通知与站内信。
const (
	CodeMessageSelectionEmpty Code = "message.selection_empty"
	CodeMessageNotFound       Code = "message.not_found"
	CodeMessageAlreadyRead    Code = "message.already_read"
	CodeMessageNothingRead    Code = "message.nothing_read"
	CodeMessageConfMissing    Code = "message.conf_missing"
	CodeNotifNotFound         Code = "notif.not_found"
	CodeNotifTitleDuplicated  Code = "notif.title_duplicated"
	CodeNotifExpireTooSoon    Code = "notif.expire_too_soon"
	CodeNotifCreateFailed     Code = "notif.create_failed"
	CodeNotifUserMissing      Code = "notif.user_missing"
)

// 站点配置与轮播图。
const (
	CodeSiteConfigMissing Code = "site.config_missing"
	CodeSiteInfoNotFound  Code = "site.info_not_found"
	CodeBannerNotFound    Code = "banner.not_found"

	CodeRuntimeConfigDBMissing    Code = "runtimeconfig.db_missing"
	CodeRuntimeConfigPathMissing  Code = "runtimeconfig.path_missing"
	CodeRuntimeConfigDecodeFailed Code = "runtimeconfig.decode_failed"

	// 首次初始化。闸门是"库里还没有管理员"：一旦有了管理员，
	// 这两个接口就永久关闭，避免成为绕过鉴权的后门。
	CodeSetupAlreadyDone          Code = "setup.already_done"
	CodeRuntimeConfigTableMissing Code = "runtimeconfig.table_missing"
)

// AI 能力。
const (
	CodeAIDisabled         Code = "ai.disabled"
	CodeAIConfigMissing    Code = "ai.config_missing"
	CodeAIUserRequired     Code = "ai.user_required"
	CodeAIContentRequired  Code = "ai.content_required"
	CodeAIContentEmpty     Code = "ai.content_empty"
	CodeAIContentTooShort  Code = "ai.content_too_short"
	CodeAINotSearchIntent  Code = "ai.not_search_intent"
	CodeAIKeywordRequired  Code = "ai.keyword_required"
	CodeAIPermissionDenied Code = "ai.permission_denied"
	CodeAIArticleMissing   Code = "ai.article_missing"
	CodeAIRequestFailed    Code = "ai.request_failed"
	CodeAIScoreMissing     Code = "ai.score_missing"
	CodeAIScoreSaveFailed  Code = "ai.score_save_failed"
	CodeAIJSONInvalid      Code = "ai.json_invalid"
	CodeAITitleRequired    Code = "ai.title_required"
	CodeAISelectionEmpty   Code = "ai.selection_empty"
	CodeAIModeUnsupported  Code = "ai.mode_unsupported"
	CodeAIStyleRequired    Code = "ai.style_required"
	CodeAIEmptyResponse    Code = "ai.empty_response"
	CodeAIResultEmpty      Code = "ai.result_empty"
	CodeAIQuestionRequired Code = "ai.question_required"

	CodeAIServiceOff     Code = "ai.service_off"
	CodeAISecretMissing  Code = "ai.secret_missing"
	CodeAIBaseURLMissing Code = "ai.base_url_missing"
	CodeAIMessageEmpty   Code = "ai.message_empty"
	CodeAIEncodeFailed   Code = "ai.encode_failed"
	CodeAIRequestBuild   Code = "ai.request_build"
	CodeAISendFailed     Code = "ai.send_failed"
	CodeAIStatusError    Code = "ai.status_error"
	CodeAIChoicesEmpty   Code = "ai.choices_empty"
	CodeAIReplyEmpty     Code = "ai.reply_empty"

	// 模型配置化之后新增：调用方拿到的是"没有可用的模型配置"，而不是字段缺失。
	CodeAIProviderMissing   Code = "ai.provider_missing"
	CodeAIVendorUnsupported Code = "ai.vendor_unsupported"
	CodeAIProviderInvalid   Code = "ai.provider_invalid"
)

// 通用工具与基础设施。
const (
	CodeCommonInvalidOrder Code = "common.invalid_order"
	CodeCommonDBRequired   Code = "common.db_required"
	CodeCdcRuleNotExist    Code = "cdc.rule_not_exist"

	CodeStorageBucketRequired    Code = "storage.bucket_required"
	CodeStorageObjectKeyRequired Code = "storage.object_key_required"
	CodeStorageSizeInvalid       Code = "storage.size_invalid"
	CodeStorageDomainMissing     Code = "storage.domain_missing"
	CodeStorageImageInfoInvalid  Code = "storage.image_info_invalid"

	CodeAuthTokenExpired    Code = "auth.token_expired"
	CodeAuthSendTooFrequent Code = "auth.send_too_frequent"
	CodeAuthRefreshMissing  Code = "auth.refresh_missing"

	CodeUsernameEmpty           Code = "username.empty"
	CodeUsernameLength          Code = "username.length"
	CodeUsernameCharset         Code = "username.charset"
	CodeUsernameUnderscoreCount Code = "username.underscore_count"
	CodeUsernameUnderscoreEdge  Code = "username.underscore_edge"
	CodeUsernameSensitive       Code = "username.sensitive"

	CodeJWTTokenEmpty      Code = "jwt.token_empty"
	CodeJWTSignAlgoInvalid Code = "jwt.sign_algo_invalid"
	CodeJWTParseFailed     Code = "jwt.parse_failed"
	CodeJWTTokenInvalid    Code = "jwt.token_invalid"
	CodeJWTConfigMissing   Code = "jwt.config_missing"
	CodeJWTConfigType      Code = "jwt.config_type"

	CodeMapsPointerRequired Code = "maps.pointer_required"
	CodeMapsStructRequired  Code = "maps.struct_required"
)

// messages 是错误码到中文文案的唯一映射。
//
// 新增错误码时**必须**在这里补一行；缺了不会 panic，Error() 会退化成码本身，
// 便于在日志里立刻看出漏配。
var messages = map[Code]string{
	CodeArticleNotFound:        "文章不存在",
	CodeArticleForbidden:       "无权操作该文章",
	CodeArticleInvalidState:    "当前状态不允许该操作",
	CodeArticleAlreadyPublic:   "文章已发布",
	CodeArticleTitleRequired:   "标题不能为空",
	CodeArticleContentRequired: "正文不能为空",
	CodeArticleVersionConflict: "文章已被他人修改，请刷新后重试",

	CodeTaxonomyTitleRequired: "名称不能为空",
	CodeTaxonomyTitleExists:   "名称重复",
	CodeCategoryNotFound:      "分类不存在",
	CodeTagNotFound:           "标签不存在",
	CodeTagDisabled:           "标签不存在或已停用",

	CodeFavoriteTitleRequired: "收藏夹名称不能为空",
	CodeFavoriteFolderMissing: "收藏夹不存在",
	CodeFavoriteFolderExists:  "收藏夹名称重复",
	CodeFavoriteFolderDenied:  "无权操作该收藏夹",
	CodeFavoriteExists:        "已收藏该文章",
	CodeFavoriteNotFound:      "尚未收藏该文章",
	CodeFavoriteSelf:          "不能收藏自己的文章",

	CodeTopLimitExceeded: "置顶数量已达上限",

	CodeReviewTaskNotFound: "审核任务不存在",
	CodeReviewTaskHandled:  "审核任务已处理",
	CodeReviewTaskDenied:   "无权处理该审核任务",

	CodeUserNotFound:          "用户不存在",
	CodeUserForbidden:         "无权操作该用户",
	CodeUserUsernameTaken:     "用户名已被使用",
	CodeUserUsernameCooldown:  "用户名每 30 天内只能更新 1 次",
	CodeUserUsernameInvalid:   "用户名格式不符合要求",
	CodeUserNicknameRequired:  "昵称不能为空",
	CodeUserNicknameTooLong:   "昵称长度超出限制",
	CodeUserAbstractTooLong:   "个人简介长度超出限制",
	CodeUserStatusInvalid:     "账号状态不合法",
	CodeUserStatusSelfLocked:  "不能修改自己的账号状态",
	CodeUserEmailTaken:        "邮箱已被使用",
	CodeUserEmailNotBound:     "账号尚未绑定邮箱",
	CodeUserEmailRegistered:   "该邮箱已注册，请直接登录或找回密码",
	CodeUserEmailUnregistered: "该邮箱还没有注册，请先注册",
	CodeUserMailTooFrequent:   "验证码发送太频繁，请稍后再试",
	CodeUserInvalidCredential: "账号或密码错误",
	CodeUserAccountDisabled:   "账号已被禁用",
	CodeUserAccountBanned:     "账号已被封禁",
	CodeUserPasswordRequired:  "请先设置密码",
	CodeUserPasswordTooShort:  "密码长度不足",
	CodeUserOldPasswordWrong:  "旧密码错误",
	CodeUserPasswordSame:      "新密码不能与旧密码相同",
	CodeUserSessionExpired:    "登录状态已过期，请重新登录",
	CodeUserTokenInvalid:      "登录凭证无效",
	CodeUserCaptchaInvalid:    "验证码错误或已过期",
	CodeUserMailSendFailed:    "邮件发送失败，请稍后重试",
	CodeUserOAuthFailed:       "第三方登录失败",
	CodeUserRegisterDisabled:  "当前站点未开放注册",

	CodeAuthRequired:       "请先登录",
	CodeAuthTooFrequent:    "登录失败次数过多，请稍后再试",
	CodeAuthRefreshInvalid: "刷新令牌无效或已过期，请重新登录",
	CodeAuthLoginDisabled:  "站点未启用该登录方式",
	CodeAuthAdminRequired:  "该账号没有后台权限",

	CodeCommentNotFound:       "评论不存在",
	CodeCommentClosed:         "该文章已关闭评论",
	CodeCommentForbidden:      "无权限删除该评论",
	CodeCommentReplyInvalid:   "回复的评论不存在",
	CodeCommentDiggChanged:    "点赞状态已变化，请刷新后重试",
	CodeCommentDiggDuplicated: "请勿重复点赞",
	CodeCommentContentEmpty:   "评论内容不能为空",

	CodeImageUnavailable:      "图片不存在或不可用",
	CodeImageUploadDisabled:   "站点未启用图片上传",
	CodeImageTaskNotFound:     "上传任务不存在",
	CodeImageTaskFailed:       "上传任务校验失败",
	CodeImageTaskProcessing:   "上传任务处理中，请稍后重试",
	CodeImageInvalid:          "图片参数不合法",
	CodeImageTooLarge:         "图片大小超出限制",
	CodeImageMismatch:         "上传对象与任务不匹配",
	CodeImageAuditInvalid:     "未知的审核结论",
	CodeImageDeleteFailed:     "删除图片失败",
	CodeImageObjectKeyMissing: "回调缺少对象 key",
	CodeImageCallbackInvalid:  "回调签名校验失败",

	CodeSearchTypeInvalid:     "搜索类型错误",
	CodeSearchSortInvalid:     "搜索排序错误",
	CodeSearchPageModeInvalid: "分页模式错误",
	CodeSearchStatusRestrict:  "当前搜索类型只能查询已发布文章",
	CodeSearchAuthorRequired:  "作者文章必须传 author_id",
	CodeSearchUnauthorized:    "请先登录",
	CodeSearchForbidden:       "权限错误",
	CodeSearchResultInvalid:   "搜索结果格式错误",

	CodeFollowSelf:           "不能关注自己",
	CodeFollowTargetNotFound: "目标用户不存在",
	CodeFollowDuplicate:      "请勿重复关注",
	CodeFollowNotFollowing:   "尚未关注该用户",
	CodeFollowListPrivate:    "关注关系未公开",
	CodeFollowConfigMissing:  "用户配置信息不存在",

	CodeChatUserNotFound:    "接收人不存在",
	CodeChatContentEmpty:    "消息内容不能为空",
	CodeChatTypeUnsupported: "不支持的消息类型",
	CodeChatStrangerOff:     "对方未开启陌生人私信",
	CodeChatNoPermission:    "权限不足",
	CodeChatSessionNotFound: "会话不存在",
	CodeChatEmptySelection:  "请选择要操作的消息或会话",
	CodeChatSendFailed:      "消息发送失败",
	CodeChatTooFrequent:     "发送过于频繁，请稍后再试",
	CodeChatSessionTooFast:  "当前会话发送过于频繁，请稍后再试",
	CodeChatWeekStranger:    "本周只允许向陌生人发送 1 条消息",
	CodeChatWeekLimit:       "本周可发送消息次数已达上限，对方回复后才可以继续发送消息",

	CodeMessageSelectionEmpty: "请选择要操作的消息或会话",
	CodeMessageNotFound:       "消息不存在",
	CodeMessageAlreadyRead:    "消息已被读取",
	CodeMessageNothingRead:    "没有可标记已读的消息",
	CodeMessageConfMissing:    "用户配置信息不存在",
	CodeNotifNotFound:         "消息不存在",
	CodeNotifTitleDuplicated:  "全局通知标题重复",
	CodeNotifExpireTooSoon:    "过期时间不能小于一天",
	CodeNotifCreateFailed:     "全局通知创建失败",
	CodeNotifUserMissing:      "用户不存在",

	CodeSiteConfigMissing: "运行时配置服务未初始化",
	CodeSiteInfoNotFound:  "站点信息不存在",
	CodeBannerNotFound:    "轮播图不存在",

	CodeSetupAlreadyDone: "站点已初始化完成，该入口已关闭",

	CodeRuntimeConfigDBMissing:    "数据库未初始化",
	CodeRuntimeConfigPathMissing:  "运行时站点默认配置路径未设置",
	CodeRuntimeConfigDecodeFailed: "运行时配置解析失败",
	CodeRuntimeConfigTableMissing: "运行时配置表不存在，请先执行数据库初始化命令：/app/server -db",

	CodeAIDisabled:         "AI 功能未启用",
	CodeAIConfigMissing:    "运行时配置服务未初始化",
	CodeAIUserRequired:     "用户 ID 不能为空",
	CodeAIContentRequired:  "内容不能为空",
	CodeAIContentEmpty:     "文章正文提取结果为空",
	CodeAIContentTooShort:  "文章内容过短，建议补充完整后再评分",
	CodeAINotSearchIntent:  "当前输入不是文章搜索意图",
	CodeAIKeywordRequired:  "搜索关键词不能为空",
	CodeAIPermissionDenied: "权限错误",
	CodeAIArticleMissing:   "文章不存在",
	CodeAIRequestFailed:    "AI 请求失败",
	CodeAIScoreMissing:     "读取文章评分失败",
	CodeAIScoreSaveFailed:  "保存文章评分失败",
	CodeAIJSONInvalid:      "AI 返回内容不是有效 JSON",
	CodeAITitleRequired:    "文章标题不能为空",
	CodeAISelectionEmpty:   "选中内容不能为空",
	CodeAIModeUnsupported:  "改写模式不支持",
	CodeAIStyleRequired:    "风格转换模式下 target_style 不能为空",
	CodeAIEmptyResponse:    "AI 返回内容为空",
	CodeAIResultEmpty:      "文章搜索结果分析为空",
	CodeAIQuestionRequired: "用户问题不能为空",

	CodeAIServiceOff:     "AI服务未开启",
	CodeAISecretMissing:  "AI SecretKey 未配置",
	CodeAIBaseURLMissing: "AI BaseURL 未配置",
	CodeAIMessageEmpty:   "消息列表不能为空",
	CodeAIEncodeFailed:   "请求体序列化失败",
	CodeAIRequestBuild:   "创建请求失败",
	CodeAISendFailed:     "发送请求失败",
	CodeAIStatusError:    "请求失败，响应状态码异常",
	CodeAIChoicesEmpty:   "AI响应无有效Choices数据",
	CodeAIReplyEmpty:     "AI回复内容为空",

	CodeAIProviderMissing:   "还没有启用的模型配置",
	CodeAIVendorUnsupported: "不支持的模型供应商",
	CodeAIProviderInvalid:   "模型配置不完整",

	CodeCommonInvalidOrder: "排序字段错误",
	CodeCommonDBRequired:   "缺少查询数据库依赖",
	CodeCdcRuleNotExist:    "规则不存在",

	CodeStorageBucketRequired:    "qiniu: bucket 不能为空",
	CodeStorageObjectKeyRequired: "qiniu: object key 不能为空",
	CodeStorageSizeInvalid:       "qiniu: size limit 必须大于 0",
	CodeStorageDomainMissing:     "qiniu: 下载域名未配置",
	CodeStorageImageInfoInvalid:  "qiniu: imageInfo 不完整",

	CodeAuthTokenExpired:    "登录已失效，请重新登录",
	CodeAuthSendTooFrequent: "请求过于频繁，请稍后再试",
	CodeAuthRefreshMissing:  "刷新令牌不存在，请重新登录",

	CodeUsernameEmpty:           "用户名不能为空",
	CodeUsernameLength:          "用户名长度必须在6-20个字符之间",
	CodeUsernameCharset:         "用户名仅支持字母、数字、下划线",
	CodeUsernameUnderscoreCount: "用户名中下划线数量最多只能有2个",
	CodeUsernameUnderscoreEdge:  "用户名不能以下划线开头或结尾",
	CodeUsernameSensitive:       "用户名包含敏感词汇",

	CodeJWTTokenEmpty:      "请登录：token 为空",
	CodeJWTSignAlgoInvalid: "token 签名算法错误",
	CodeJWTParseFailed:     "token 解析错误",
	CodeJWTTokenInvalid:    "token 无效",
	CodeJWTConfigMissing:   "jwt 配置缺失",
	CodeJWTConfigType:      "jwt 配置类型错误",

	CodeMapsPointerRequired: "src 和 dest 必须是指针类型",
	CodeMapsStructRequired:  "src 和 dest 指针必须指向结构体类型",
}

// Error 是带错误码的应用错误。
type Error struct {
	code  Code
	cause error
}

// New 用错误码创建一个错误。
func New(code Code) *Error { return &Error{code: code} }

// Wrap 在保留原因的同时附加错误码。
func Wrap(code Code, cause error) *Error { return &Error{code: code, cause: cause} }

// Code 返回错误码。
func (e *Error) Code() Code { return e.code }

// Error 返回面向用户的文案；未配置文案时返回码本身，便于发现漏配。
func (e *Error) Error() string {
	if msg, ok := messages[e.code]; ok {
		return msg
	}
	return string(e.code)
}

// Unwrap 支持 errors.Is / errors.As 穿透到原因。
func (e *Error) Unwrap() error { return e.cause }

// Is 让 errors.Is(err, apperr.New(code)) 也能成立——
// 否则调用点必须持有同一个实例，跨包比较容易写错。
func (e *Error) Is(target error) bool {
	var other *Error
	if errors.As(target, &other) {
		return other.code == e.code
	}
	return false
}

// CodeOf 从错误链里取错误码。
func CodeOf(err error) (Code, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.code, true
	}
	return "", false
}

// IsCode 判断错误链里是否是某个错误码。
func IsCode(err error, code Code) bool {
	got, ok := CodeOf(err)
	return ok && got == code
}

// Message 返回错误码对应的文案（供 adapter 兜底使用）。
func Message(code Code) string {
	if msg, ok := messages[code]; ok {
		return msg
	}
	return string(code)
}
