package apperr_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strconv"
	"strings"
	"testing"

	"myblogx/internal/apperr"
)

// declaredCodes 枚举非测试源码里声明的全部 `Code = "..."` 常量。
//
// 为什么从源码枚举，而不是在测试里再抄一份错误码清单：抄一份的话，新增错误码
// 时两边会一起忘，这条守卫就永远绿 —— 与项目里"守卫检查的符号根本不存在"是
// 同一类失效。这里只认常量声明本身，不解析任何行为。
func declaredCodes(t *testing.T) []apperr.Code {
	t.Helper()

	const dir, pkgName = ".", "apperr"
	pkgs, err := parser.ParseDir(token.NewFileSet(), dir, func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("解析 %s 失败: %v", dir, err)
	}
	pkg, ok := pkgs[pkgName]
	if !ok {
		t.Fatalf("%s 里没有包 %s（枚举失败，别让这条守卫变成永真）", dir, pkgName)
	}

	var codes []apperr.Code
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			gen, isGen := decl.(*ast.GenDecl)
			if !isGen || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value, isValue := spec.(*ast.ValueSpec)
				if !isValue || len(value.Values) != 1 {
					continue
				}
				ident, isIdent := value.Type.(*ast.Ident)
				if !isIdent || ident.Name != "Code" {
					continue
				}
				lit, isLit := value.Values[0].(*ast.BasicLit)
				if !isLit || lit.Kind != token.STRING {
					continue
				}
				raw, unquoteErr := strconv.Unquote(lit.Value)
				if unquoteErr != nil {
					t.Fatalf("错误码字面量无法解析: %v", unquoteErr)
				}
				codes = append(codes, apperr.Code(raw))
			}
		}
	}
	return codes
}

// serviceFallbackCodes 是**有意**按 ProtocolService（通用服务异常码）返回的错误码。
//
// protocols 与 messages 是同键双表：漏登记 messages 会退化成码本身（一眼看得出），
// 漏登记 protocols 却会**静默**落到 ProtocolService —— 前端按协议码分支的逻辑就
// 此失效，没有任何报错。所以新增错误码必须二选一：
//   - 在 protocol.go 的 protocols 表里登记协议类别；
//   - 或者加进这张表，明确承认"它按服务异常返回"。
//
// 这张表只允许收缩：删掉一条意味着那个码登记了协议类别。新增一条必须在提交
// 说明里写清为什么它属于服务异常，而不是参数 / 登录态问题。
var serviceFallbackCodes = []apperr.Code{
	// 分组功能未就绪（装配层没提供分组仓储）：这不是用户能修的问题，
	// 也不该暴露内部装配细节，所以按通用服务异常返回。
	apperr.CodeGroupWiring,
	// 主密钥缺失同样是运维侧配置问题（部署时忘了设环境变量），
	// 不该让用户看到"配置解析失败"这种指向数据损坏的文案。
	apperr.CodeRuntimeConfigSecretUnavailable,
	// 文章主体。
	apperr.CodeArticleNotFound,
	apperr.CodeArticleForbidden,
	apperr.CodeArticleInvalidState,
	apperr.CodeArticleAlreadyPublic,
	apperr.CodeArticleTitleRequired,
	apperr.CodeArticleContentRequired,
	apperr.CodeArticleVersionConflict,
	apperr.CodeArticlePublishDisabled,
	apperr.CodeArticleDraftDisabled,
	apperr.CodeArticleReviewReasonRequired,
	// #308：驳回原因从"必填一句话"变成"必选一条固定枚举 + 可选一句话"。
	// 两个新码都是审核员当场能改的动作（重选 / 精简），所以按服务兜底类别登记。
	apperr.CodeArticleReviewReasonInvalid,
	apperr.CodeArticleReviewReasonTooLong,
	// 投稿配额用完（票 304）：不是"你传错了参数"，调用方也修不了；
	// 它只是今天没次数了，明天自然恢复。因此按服务兜底类别，不伪装成校验失败。
	apperr.CodeArticleSubmitQuotaExceeded,
	// 质量闸门（票 306）：两类都不是参数问题。score_unavailable 是上游模型
	// 没给出可用结论（作者改稿修不好它，只能等）；quality_rejected 是系统按
	// 配置给出了"不够好"的结论，作者能改，但它的下一步动作是改稿而不是修参数。
	// quality_gate_requires_ai 是管理员保存配置时撞上的前提缺失，同样不是参数错误。
	apperr.CodeArticleScoreUnavailable,

	// 分类与标签。
	apperr.CodeTaxonomyTitleRequired,
	apperr.CodeTaxonomyTitleExists,
	apperr.CodeCategoryNotFound,
	apperr.CodeTagNotFound,
	apperr.CodeTagDisabled,
	// 标签删除门禁（票 322）：不是"你传错了参数"，是"这条标签现在不能删"。
	// 管理员的正确动作是改用停用，重发一次请求修不好它，所以按服务兜底类别，
	// 与同一组其它标签错误码保持一致（前端只读文案，不按 1001/1002 分支）。
	apperr.CodeTagInUse,
	// 标签拖拽排序（票 346）：并发改名 / 删除让提交的 ID 全集与库里不一致。
	// 前端的正确动作是刷新后重排，重发同一个请求修不好它，与项目换序同一类别。
	apperr.CodeTagOrderMismatch,

	// 收藏。
	apperr.CodeFavoriteTitleRequired,
	apperr.CodeFavoriteFolderMissing,
	apperr.CodeFavoriteFolderExists,
	apperr.CodeFavoriteFolderDenied,
	apperr.CodeFavoriteExists,
	apperr.CodeFavoriteNotFound,
	apperr.CodeFavoriteSelf,

	// 置顶。
	apperr.CodeTopLimitExceeded,
	apperr.CodeHotTopLimitExceeded,
	apperr.CodeHotTopNotPinnable,
	apperr.CodeHotTopOrderMismatch,

	// 合集。
	apperr.CodeCollectionTitleRequired,
	apperr.CodeCollectionTitleTooLong,
	apperr.CodeCollectionNotFound,

	// 项目。
	apperr.CodeProjectTitleRequired,
	apperr.CodeProjectTitleTooLong,
	apperr.CodeProjectNotFound,
	apperr.CodeProjectOrderMismatch,

	// 目录树（合集 / 项目共用的一张树表）。
	apperr.CodeOutlineTitleRequired,
	apperr.CodeOutlineTitleTooLong,
	apperr.CodeOutlineArticleDuplicated,
	apperr.CodeOutlineArticleNotIncludable,
	apperr.CodeOutlineStructureInvalid,
	apperr.CodeOutlineNodeMissing,

	// 审核任务。
	apperr.CodeReviewTaskNotFound,
	apperr.CodeReviewTaskHandled,
	apperr.CodeReviewTaskDenied,

	// 用户资料与管理。
	apperr.CodeUserNotFound,
	apperr.CodeUserForbidden,
	apperr.CodeUserUsernameTaken,
	apperr.CodeUserUsernameCooldown,
	apperr.CodeUserUsernameInvalid,
	apperr.CodeUserNicknameRequired,
	apperr.CodeUserNicknameTooLong,
	apperr.CodeUserAbstractTooLong,
	apperr.CodeUserStatusInvalid,
	apperr.CodeUserStatusSelfLocked,
	apperr.CodeUserEmailTaken,
	apperr.CodeUserEmailNotBound,
	apperr.CodeUserEmailRegistered,
	apperr.CodeUserEmailUnregistered,
	apperr.CodeUserMailTooFrequent,
	apperr.CodeUserPhoneTaken,
	apperr.CodeUserPhoneInvalid,
	apperr.CodeUserSmsTooFrequent,
	apperr.CodeUserSmsSendFailed,
	apperr.CodeUserSmsVerifyFailed,
	// 短信凭证没填是运维侧的事（与 CodeRuntimeConfigSecretUnavailable 同一类），
	// 用户改不了任何东西，按通用服务异常返回；文案已指向「站点 → 短信」页面。
	apperr.CodeUserSmsNotConfigured,
	apperr.CodeUserInvalidCredential,
	apperr.CodeUserAccountDisabled,
	apperr.CodeUserAccountBanned,
	apperr.CodeUserPasswordRequired,
	apperr.CodeUserPasswordAlreadySet,
	apperr.CodeUserPasswordTooShort,
	apperr.CodeUserOldPasswordWrong,
	apperr.CodeUserPasswordSame,
	apperr.CodeUserCaptchaInvalid,
	apperr.CodeUserMailSendFailed,
	apperr.CodeUserOAuthFailed,
	apperr.CodeUserRegisterDisabled,

	// 认证与会话。
	apperr.CodeAuthTooFrequent,
	apperr.CodeAuthLoginDisabled,
	apperr.CodeAuthAdminRequired,

	// 评论。
	apperr.CodeCommentNotFound,
	apperr.CodeCommentClosed,
	apperr.CodeCommentForbidden,
	apperr.CodeCommentReplyInvalid,
	apperr.CodeCommentDiggChanged,
	apperr.CodeCommentDiggDuplicated,
	apperr.CodeCommentContentEmpty,

	// 图片与对象存储。
	apperr.CodeImageUnavailable,
	apperr.CodeImageUploadDisabled,
	apperr.CodeImageTaskNotFound,
	apperr.CodeImageTaskFailed,
	apperr.CodeImageTaskProcessing,
	apperr.CodeImageInvalid,
	apperr.CodeImageTooLarge,
	apperr.CodeImageMismatch,
	apperr.CodeImageAuditInvalid,
	apperr.CodeImageDeleteFailed,
	apperr.CodeImageObjectKeyMissing,
	apperr.CodeImageCallbackInvalid,
	apperr.CodeImagePurposeInvalid,
	apperr.CodeImageRatioMismatch,

	// 搜索。
	apperr.CodeSearchTypeInvalid,
	apperr.CodeSearchSortInvalid,
	apperr.CodeSearchPageModeInvalid,
	apperr.CodeSearchStatusRestrict,
	apperr.CodeSearchAuthorRequired,
	apperr.CodeSearchUnauthorized,
	apperr.CodeSearchForbidden,
	apperr.CodeSearchResultInvalid,
	apperr.CodeSearchQueryInvalid,

	// 关注关系。
	apperr.CodeFollowSelf,
	apperr.CodeFollowTargetNotFound,
	apperr.CodeFollowDuplicate,
	apperr.CodeFollowNotFollowing,
	apperr.CodeFollowListPrivate,
	apperr.CodeFollowConfigMissing,

	// 私信。
	apperr.CodeChatUserNotFound,
	apperr.CodeChatContentEmpty,
	apperr.CodeChatTypeUnsupported,
	apperr.CodeChatImageNotOwned,
	apperr.CodeChatBlocked,
	apperr.CodeChatStrangerOff,
	apperr.CodeChatRecipientStrangerOff,
	apperr.CodeChatNoPermission,
	apperr.CodeChatSessionNotFound,
	apperr.CodeChatEmptySelection,
	apperr.CodeChatSendFailed,
	apperr.CodeChatTooFrequent,
	apperr.CodeChatSessionTooFast,
	apperr.CodeChatDailyLimit,
	apperr.CodeChatWeekStranger,
	apperr.CodeChatWeekLimit,

	// 通知与站内信。
	apperr.CodeNotifCreateFailed,

	// 站点配置与轮播图。
	apperr.CodeSiteConfigMissing,
	apperr.CodeSiteInfoNotFound,
	apperr.CodeBannerNotFound,
	apperr.CodeSidebarCardKindInvalid,
	apperr.CodeSidebarCardLimitExceeded,
	apperr.CodeRuntimeConfigDBMissing,
	apperr.CodeRuntimeConfigPathMissing,
	apperr.CodeRuntimeConfigDecodeFailed,
	apperr.CodeSetupAlreadyDone,
	apperr.CodeRuntimeConfigTableMissing,
	apperr.CodeSiteAIDailyLimitInvalid,

	// AI 能力。
	apperr.CodeAIDisabled,
	apperr.CodeAIConfigMissing,
	apperr.CodeAIUserRequired,
	apperr.CodeAIContentRequired,
	apperr.CodeAIContentEmpty,
	apperr.CodeAIContentTooShort,
	apperr.CodeAINotSearchIntent,
	apperr.CodeAIKeywordRequired,
	apperr.CodeAIPermissionDenied,
	apperr.CodeAIArticleMissing,
	apperr.CodeAIRequestFailed,
	apperr.CodeAIScoreMissing,
	apperr.CodeAIScoreSaveFailed,
	apperr.CodeAIJSONInvalid,
	apperr.CodeAITitleRequired,
	apperr.CodeAISelectionEmpty,
	apperr.CodeAIModeUnsupported,
	apperr.CodeAIStyleRequired,
	apperr.CodeAIEmptyResponse,
	apperr.CodeAIResultEmpty,
	apperr.CodeAIQuestionRequired,
	apperr.CodeAIServiceOff,
	apperr.CodeAISecretMissing,
	apperr.CodeAIBaseURLMissing,
	apperr.CodeAIMessageEmpty,
	apperr.CodeAIEncodeFailed,
	apperr.CodeAIRequestBuild,
	apperr.CodeAISendFailed,
	apperr.CodeAIStatusError,
	apperr.CodeAIChoicesEmpty,
	apperr.CodeAIReplyEmpty,
	apperr.CodeAIProviderMissing,
	apperr.CodeAIVendorUnsupported,
	apperr.CodeAIProviderInvalid,
	apperr.CodeAIPolicySkillMissing,
	apperr.CodeAIDailyLimitExceeded,
	apperr.CodeAIQuotaUnavailable,

	// 通用工具与基础设施。
	apperr.CodeCommonServiceFailed,
	apperr.CodeCommonDBRequired,
	apperr.CodeCdcRuleNotExist,
	apperr.CodeStorageBucketRequired,
	apperr.CodeStorageObjectKeyRequired,
	apperr.CodeStorageSizeInvalid,
	apperr.CodeStorageDomainMissing,
	apperr.CodeStorageImageInfoInvalid,
	apperr.CodeAuthSendTooFrequent,
	apperr.CodeAuthRefreshMissing,
	apperr.CodeUsernameEmpty,
	apperr.CodeUsernameLength,
	apperr.CodeUsernameCharset,
	apperr.CodeUsernameUnderscoreCount,
	apperr.CodeUsernameUnderscoreEdge,
	apperr.CodeUsernameSensitive,
	apperr.CodeJWTTokenEmpty,
	apperr.CodeJWTSignAlgoInvalid,
	apperr.CodeJWTParseFailed,
	apperr.CodeJWTTokenInvalid,
	apperr.CodeJWTConfigMissing,
	apperr.CodeJWTConfigType,
	apperr.CodeMapsPointerRequired,
	apperr.CodeMapsStructRequired,
}

// TestEveryCodeHasMessageAndProtocolDecision 遍历源码里声明的全部错误码，
// 逐个检查两张同键表都做了决定。
func TestEveryCodeHasMessageAndProtocolDecision(t *testing.T) {
	t.Parallel()

	codes := declaredCodes(t)
	if len(codes) < 100 {
		t.Fatalf("只枚举到 %d 个错误码，枚举逻辑可能失效（别让这条守卫变成永真）", len(codes))
	}

	allowed := make(map[apperr.Code]bool, len(serviceFallbackCodes))
	for _, code := range serviceFallbackCodes {
		if allowed[code] {
			t.Fatalf("serviceFallbackCodes 里 %s 重复", code)
		}
		allowed[code] = true
	}

	seen := make(map[apperr.Code]bool, len(codes))
	for _, code := range codes {
		if seen[code] {
			t.Errorf("错误码 %s 重复声明（同一码两处定义，行为取决于谁后注册）", code)
			continue
		}
		seen[code] = true

		if msg := apperr.Message(code); msg == "" || msg == string(code) {
			t.Errorf("%s 没有中文文案（messages 表漏登记，已退化成码本身）", code)
		}

		switch kind := apperr.ProtocolOf(apperr.New(code)); kind {
		case apperr.ProtocolValidation, apperr.ProtocolAuth:
			if allowed[code] {
				t.Errorf("%s 已在 protocols 登记协议类别，却还留在 serviceFallbackCodes 里（豁免过期，请删掉）", code)
			}
		case apperr.ProtocolService:
			if !allowed[code] {
				t.Errorf("%s 的协议类别落到了默认兜底：要么在 protocol.go 的 protocols 表登记，要么加进 serviceFallbackCodes 并说明理由", code)
			}
		default:
			t.Errorf("%s 的协议类别未知: %d", code, kind)
		}
	}

	for _, code := range serviceFallbackCodes {
		if !seen[code] {
			t.Errorf("serviceFallbackCodes 里的 %s 不是已声明的错误码（豁免过期，请删掉）", code)
		}
	}
}
