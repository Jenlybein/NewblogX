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

	// 通知与站内信：调用方能修正的失败。
	// 评论命中阻断词：用户改一改就能过，属于"输入不合规"，与内容为空同类。
	CodeCommentBlocked:        ProtocolValidation,
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

	// 合集 / 项目分组（ADR-0041）：四条都是调用方能看懂并放弃 / 修正的失败，
	// 归 1001 而不是 1002（后者会让前端以为后端坏了）。
	// order_mismatch 是"你手上的列表过期了，刷新后重排"，属于可恢复的校验失败。
	CodeGroupTitleRequired: ProtocolValidation,
	CodeGroupTitleTooLong:  ProtocolValidation,
	CodeGroupNotFound:      ProtocolValidation,
	CodeGroupOrderMismatch: ProtocolValidation,
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
