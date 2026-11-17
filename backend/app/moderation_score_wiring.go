package app

import (
	"context"
	"fmt"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/idgen"

	moderationhttp "myblogx/internal/moderation/adapter/http"
	moderationmysql "myblogx/internal/moderation/adapter/mysql"
	moderationapp "myblogx/internal/moderation/app"
	moderationdomain "myblogx/internal/moderation/domain"
	notificationapp "myblogx/internal/notification/app"
	"myblogx/internal/platform/http/router"
	userapp "myblogx/internal/user/app"
)

// 本文件是违规计分（ADR-0035）的装配适配：
//   - 把账号模块的窄接口接成计分的处罚端口；
//   - 把站内通知接成计分的通知端口；
//   - 把媒体 / 过滤两个模块的上报口接成它们的 ViolationRecorder。
//
// 注意与同目录的 moderation_wiring.go 区分：那个是**文章机审评分**的装配
// （票 311，AI 评分层），本文件是**用户违规计分**，两套东西不要混。

// moderationService 是全局唯一的违规计分服务（装配期构造一次）。
var moderationService *moderationapp.Service

// WireModeration 构造违规计分服务并接上两个上报口。
//
// 必须在 WireMedia / WireFilter / WireUser / WireNotification 之后调用：
// 那些模块的服务要先存在。
func WireModeration(infra *Infra) *moderationapp.Service {
	if infra == nil || infra.UserService == nil {
		return nil
	}

	repo := moderationmysql.New(infra.DB, moderationIDs{})
	moderationService = moderationapp.New(
		repo,
		moderationAccount{users: infra.UserService},
		moderationNotifier{},
		realClock{},
		infra.Logger,
	)

	// 处理器：登录用户看自己的记录并申诉，管理员处置。
	router.SetModerationHandler(moderationhttp.NewHandler(moderationService))

	// 申诉仓储（ADR-0035）：用户提交、管理员处置。
	moderationService.SetAppeals(moderationmysql.NewAppealRepo(infra.DB, moderationIDs{}))

	// 接上报口：图片违规与关键词命中都会落到同一个账本上。
	if infra.MediaService != nil {
		infra.MediaService.SetViolationRecorder(imageViolationRecorder{moderation: moderationService})
	}
	if infra.Filter != nil {
		infra.Filter.SetViolationRecorder(keywordViolationRecorder{moderation: moderationService})
	}
	return moderationService
}

// Moderation 返回装配好的计分服务（可能是 nil）。
func Moderation() *moderationapp.Service { return moderationService }

// moderationIDs 用雪花生成事件 / 申诉 ID。
type moderationIDs struct{}

// NextID 返回下一个 ID。
func (moderationIDs) NextID() (ctype.ID, error) {
	next, err := idgen.Next()
	if err != nil {
		return "", err
	}
	return next, nil
}

// moderationAccount 把 user 用例服务适配成计分的处罚端口。
type moderationAccount struct {
	users *userapp.Service
}

// IsAdmin 管理员豁免：账号上的分数与封禁对站长自己毫无意义。
func (a moderationAccount) IsAdmin(ctx context.Context, userID ctype.ID) (bool, error) {
	return a.users.IsAdminAccount(ctx, userID)
}

// IsMuted 当前是否处在禁言期（"封禁期内再违规升一档"要用）。
func (a moderationAccount) IsMuted(ctx context.Context, userID ctype.ID) (bool, error) {
	return a.users.IsAccountMuted(ctx, userID)
}

// Mute 由系统施加禁言。
func (a moderationAccount) Mute(ctx context.Context, userID ctype.ID, days int) error {
	return a.users.PunishMute(ctx, userID, days)
}

// Unmute 解除禁言（申诉通过后把后果退回去）。
func (a moderationAccount) Unmute(ctx context.Context, userID ctype.ID) error {
	return a.users.RelieveMute(ctx, userID)
}

// moderationNotifier 把站内通知适配成计分的通知端口。
type moderationNotifier struct{}

// NotifySystem 发一条系统通知。
func (moderationNotifier) NotifySystem(ctx context.Context, receiver ctype.ID, content, linkTitle, linkHref string) error {
	if notificationService == nil {
		return fmt.Errorf("通知服务未装配")
	}
	return notificationService.NotifySystem(ctx, notificationapp.SystemNotice{
		Receiver:  receiver.Uint64(),
		Content:   content,
		LinkTitle: linkTitle,
		LinkHref:  linkHref,
	})
}

// imageViolationRecorder 把"图片违规"翻译成一条计分事件。
type imageViolationRecorder struct {
	moderation *moderationapp.Service
}

// RecordImageViolation 记一条图片违规事件（+20）。
func (r imageViolationRecorder) RecordImageViolation(ctx context.Context, userID, imageID ctype.ID, objectKey string) error {
	if r.moderation == nil {
		return nil
	}
	_, err := r.moderation.Record(ctx, moderationapp.RecordInput{
		UserID:  userID,
		Level:   moderationdomain.LevelImage,
		Source:  moderationdomain.SourceMachine,
		RefType: "image",
		RefID:   imageID,
		// 证据只写对象 key：它是排查线索，且不含用户内容，不会连带动词表泄漏。
		Evidence: fmt.Sprintf("图片审核判定违规，对象 %s", objectKey),
	})
	return err
}

// keywordViolationRecorder 把"关键词命中"翻译成一条计分事件。
type keywordViolationRecorder struct {
	moderation *moderationapp.Service
}

// RecordKeywordViolation 记一条关键词事件（+5）。
//
// 幂等键用命中记录 ID：一次请求只产生一条命中记录（一次命中多个词也只记一条），
// 所以"同一请求命中多个词"天然只计一次分。
func (r keywordViolationRecorder) RecordKeywordViolation(ctx context.Context, userID, hitID ctype.ID, surface string) error {
	if r.moderation == nil {
		return nil
	}
	_, err := r.moderation.Record(ctx, moderationapp.RecordInput{
		UserID:  userID,
		Level:   moderationdomain.LevelKeyword,
		Source:  moderationdomain.SourceMachine,
		RefType: "filter_hit",
		RefID:   hitID,
		// 不写命中词：事件与通知里出现词表等于把"该绕开哪些词"公开（ADR-0032 同口径）。
		Evidence: fmt.Sprintf("内容过滤命中（文本面 %s）", surface),
	})
	return err
}
