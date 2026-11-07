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
	CodeCommonInvalidOrder:        ProtocolValidation,
	CodeCommonInvalidArgument:     ProtocolValidation,
	CodeCommonBindMissing:         ProtocolValidation,
	// 质量闸门：不达标与"开启闸门时 AI 不可用"都是可以按提示修正的失败，
	// 用 1001 而不是 1002（后者会让调用方以为后端坏了）。
	CodeArticleQualityRejected:       ProtocolValidation,
	CodeArticleQualityGateRequiresAI: ProtocolValidation,
	CodeMessageSelectionEmpty:     ProtocolValidation,
	CodeMessageIDAndTypeEmpty:     ProtocolValidation,
	CodeMessageNotFound:           ProtocolValidation,
	CodeMessageAlreadyRead:        ProtocolValidation,
	CodeMessageNothingRead:        ProtocolValidation,
	CodeMessageConfMissing:        ProtocolValidation,
	CodeNotifForbidden:            ProtocolValidation,
	CodeNotifAdminRequired:        ProtocolValidation,
	CodeNotifInvalidArgument:      ProtocolValidation,
	CodeNotifReadSelectionEmpty:   ProtocolValidation,
	CodeNotifDeleteSelectionEmpty: ProtocolValidation,
	CodeNotifDeleteNotFound:       ProtocolValidation,
	CodeNotifNotFound:             ProtocolValidation,
	CodeNotifTitleDuplicated:      ProtocolValidation,
	CodeNotifExpireTooSoon:        ProtocolValidation,
	CodeNotifUserMissing:          ProtocolValidation,
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
