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
