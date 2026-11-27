package apperr

// ProtocolKind 是错误码在 HTTP 协议里的类别。
//
// 数值协议码（1001 / 1002 / 1003）只由共享响应层定义；这里只回答
// 「这个错误码属于哪一类」。判定只看错误值本身，不看它是从哪个分支返回的，
// 因此同一个领域错误不可能再落到两种协议码上。
type ProtocolKind uint8

const (
	// ProtocolValidation 是参数 / 业务规则失败：调用方可以自己修正输入。
	ProtocolValidation ProtocolKind = iota + 1
	// ProtocolAuth 是登录态不可用：调用方应刷新令牌或重新登录。
	ProtocolAuth
	// ProtocolService 是服务异常，也是未知错误的兜底类别。
	ProtocolService
)

// protocols 是错误码 → 协议类别的唯一映射。
//
// 未登记的码按服务异常处理：不吞错误，也不把内部报错原文写进响应。
var protocols = map[Code]ProtocolKind{
	// 登录态：与"参数错误"分开，前端据此决定要不要刷新令牌。
	CodeAuthRequired:       ProtocolAuth,
	CodeAuthTokenExpired:   ProtocolAuth,
	CodeAuthRefreshInvalid: ProtocolAuth,
	CodeUserTokenInvalid:   ProtocolAuth,
	CodeUserSessionExpired: ProtocolAuth,

	// 禁言（票 399 / ADR-0032 决策 8）：被禁言者账号仍然有效（能登录、浏览、
	// 点赞、收藏），只是发布类动作被拒；到期自动恢复，不做任何人工动作。
	// 这是调用方看得懂、等得到的业务校验失败，归 1001 而不是 1002（后者会让
	// 前端以为后端坏了、反复重试）。
	CodeUserMuted:           ProtocolValidation,
	CodeUserMuteDaysInvalid: ProtocolValidation,

	// 通知与站内信：调用方能修正的失败。
	CodeCommonInvalidOrder:    ProtocolValidation,
	CodeCommonInvalidArgument: ProtocolValidation,
	CodeCommonBindMissing:     ProtocolValidation,
	// 质量闸门：不达标与"开启闸门时 AI 不可用"都是可以按提示修正的失败，
	// 用 1001 而不是 1002（后者会让调用方以为后端坏了）。
	// risk_rejected 是内容检查未通过的业务拒绝，同样归 1001，
	// 但作者只看到固定文案，不给命中词与等级。
	CodeArticleQualityRejected:       ProtocolValidation,
	CodeArticleQualityGateRequiresAI: ProtocolValidation,
	CodeArticleRiskRejected:          ProtocolValidation,
	// 强制退回草稿没填原因（管理员纠错）：后台补一句话就能过，属于调用方
	// 自己修正输入即可解决的失败，归 1001；归 1002 会让运营以为接口坏了。
	CodeArticleForceDemoteReasonRequired: ProtocolValidation,
	CodeMessageSelectionEmpty:        ProtocolValidation,
	CodeMessageIDAndTypeEmpty:        ProtocolValidation,
	CodeMessageNotFound:              ProtocolValidation,
	CodeMessageAlreadyRead:           ProtocolValidation,
	CodeMessageNothingRead:           ProtocolValidation,
	CodeMessageConfMissing:           ProtocolValidation,
	CodeNotifForbidden:               ProtocolValidation,
	CodeNotifAdminRequired:           ProtocolValidation,
	CodeNotifInvalidArgument:         ProtocolValidation,
	CodeNotifReadSelectionEmpty:      ProtocolValidation,
	CodeNotifDeleteSelectionEmpty:    ProtocolValidation,
	CodeNotifDeleteNotFound:          ProtocolValidation,
	CodeNotifNotFound:                ProtocolValidation,
	CodeNotifTitleDuplicated:         ProtocolValidation,
	CodeNotifExpireTooSoon:           ProtocolValidation,
	CodeNotifUserMissing:             ProtocolValidation,

	// 移除粉丝（票 327）：对方本来就不是我的粉丝，是调用方能看懂并放弃的失败，
	// 归 1001 而不是 1002（后者会让前端以为后端坏了）。
	// 相邻的几个 follow.* 码是更早的一批，仍走默认兜底 —— 这里只登记新码，
	// 不做顺手改判，免得同一批接口的行为在无关改动里漂移。
	CodeFollowFanNotFound: ProtocolValidation,

	// 合集 / 项目分组（ADR-0019）：四条都是调用方能看懂并放弃 / 修正的失败，
	// 归 1001 而不是 1002（后者会让前端以为后端坏了）。
	// order_mismatch 是"你手上的列表过期了，刷新后重排"，属于可恢复的校验失败。
	CodeGroupTitleRequired: ProtocolValidation,
	CodeGroupTitleTooLong:  ProtocolValidation,
	CodeGroupNotFound:      ProtocolValidation,
	CodeGroupOrderMismatch: ProtocolValidation,

	// 内容过滤词表（ADR-0032）：保存的是运营填错的词条/档位/类目，改一改就能过，
	// 归 1001 而不是 1002（后者会让后台以为服务坏了、不敢重试）。
	CodeSiteContentFilterInvalid: ProtocolValidation,

	// 图形验证码段（ADR-0033）：存的是运营填错的类型/尺寸/有效期/字符集/颜色，
	// 改一改就能过 —— 归 1001。归 1002 会让后台以为服务坏了，明明给的是哪一项越界
	// （详情见 UserMessage）却只看到「服务异常」。
	// 对象存储段（ADR-0034）：填错的是样式名/分隔符/签名有效期/审核开关，
	// 改一改就能过 —— 同样归 1001。归 1002 会让运营以为存储服务坏了，
	// 而真正的原因（哪个字段越界）就在 UserMessage 里。
	CodeSiteStorageInvalid: ProtocolValidation,

	// 违规事件与申诉（ADR-0035）：id 传错 / 超窗口 / 重复申诉都是"改一改就能过"，
	// 归 1001；归 1002 会让前端以为服务坏了、不敢让用户重试。
	CodeModerationEventNotFound: ProtocolValidation,
	CodeModerationAppealInvalid: ProtocolValidation,

	CodeSiteCaptchaInvalid: ProtocolValidation,

	// 关键词风险页的筛选参数（ADR-0032）：后台传了不认识的文本面或状态，
	// 换个值就能过，同属调用方能修正的失败。
	CodeFilterSurfaceInvalid: ProtocolValidation,
	// 昵称/用户名命中关键词（user.name_blocked）：调用方换个名字就能过，
	// 与上面几条同属"改一个值即可修复"，所以是参数类而不是服务异常。
	CodeProfileNameBlocked:  ProtocolValidation,
	CodeFilterStatusInvalid: ProtocolValidation,

	// 关键词风险的处置动作（ADR-0032 决策 7 / 票 398）：记录不存在、动作写错、
	// 备注超长、批量没选、私信面没内容 —— 都是后台换一下操作就能过的失败，归 1001。
	CodeFilterHitNotFound:      ProtocolValidation,
	CodeFilterActionInvalid:    ProtocolValidation,
	CodeFilterNoteTooLong:      ProtocolValidation,
	CodeFilterBatchEmpty:       ProtocolValidation,
	CodeFilterSurfaceNoContent: ProtocolValidation,
	// 处置端口没装配（filter.dispose_unavailable）**不**登记在这里：它明确属于
	// 服务异常，按仓库约定登记在 protocol_test.go 的 serviceFallbackCodes 并写明理由。
}

// ProtocolOf 从错误值取协议类别。
//
// 没有错误码的未知错误按服务异常处理：它仍然是一次失败（不吞），
// 但共享响应层会换成通用文案，绝不带出内部报错原文。
func ProtocolOf(err error) ProtocolKind {
	code, ok := CodeOf(err)
	if !ok {
		return ProtocolService
	}
	if kind, ok := protocols[code]; ok {
		return kind
	}
	return ProtocolService
}
