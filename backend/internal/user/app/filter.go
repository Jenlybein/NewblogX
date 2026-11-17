package app

import (
	"context"
	"strings"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/user/domain"
)

// 昵称与个人简介的内容过滤（票 397 / ADR-0032 决策 3）。
//
// 这两项是本次事故里**唯一被判定为"发布传播"的文本面**：昵称会渲染在公开个人
// 主页、评论区作者名上，简介会渲染在公开主页与搜索结果里，而它们在改造前只有
// 长度校验、没有任何过滤。所以它们必须和文章 / 评论走同一层过滤，只是处置不同 ——
// 命中就替换后保存（昵称与简介是"用户资料"，拦下来不让他改昵称没有意义）。
//
// 端口由本包定义、装配层实现：用户模块只知道"这是我的昵称 / 简介"，把文本归到
// 哪个面属于内容过滤模块（面枚举在 bridge 里钉死）。

// FilterText 是一段要在落库前过滤的用户资料文本。
type FilterText struct {
	// UserID 是这段文本的主人，命中记录按它统计（风险页的"命中用户 Top"）。
	UserID domain.UserID
	// ContentID 是这条资料所属的用户 ID：昵称与简介没有独立的内容 ID，
	// 处置动作里的"重置资料"要靠它找回用户。与 UserID 同值，单独留一个字段
	// 是为了让命中记录的语义与另外五个面一致（每个面都要能指回内容）。
	ContentID ctype.ID
	Text      string
}

// FilteredText 是过滤结果。
type FilteredText struct {
	// Masked 是替换后的文本（与输入等长，按 rune 计）。
	Masked string
	// FilteredCount 是实际被替换的处数。
	FilteredCount int
}

// NicknameTextFilterer 在全站唯一的内容过滤层里替换**昵称**并记一条关键词风险。
type NicknameTextFilterer interface {
	FilterNicknameText(ctx context.Context, in FilterText) (FilteredText, error)
}

// AbstractTextFilterer 在全站唯一的内容过滤层里替换**个人简介**并记一条关键词风险。
//
// 与昵称分成两个接口：两个面在风险页上是筛选项，接错面会让管理员在"昵称"里
// 看到简介的命中记录。分成两个方法后，接错就是编译错误。
type AbstractTextFilterer interface {
	FilterAbstractText(ctx context.Context, in FilterText) (FilteredText, error)
}

// filterProfile 过滤本次更新里出现的昵称与简介，返回替换后的更新指令。
//
// 只处理**非 nil** 的字段：nil 表示"这次不改"，替换成空串会把"不改"写成"清空"。
// 失败方向是 fail-open（ADR-0032 决策 11）：过滤器报错或没装配时保留原文，
// 用户照常保存资料 —— 审计层出问题不该演变成"改不了昵称"。
func (s *Service) filterProfile(
	ctx context.Context, userID domain.UserID, update domain.ProfileUpdate,
) domain.ProfileUpdate {
	if update.Nickname != nil {
		if masked, ok := s.filterOneProfileText(
			ctx, userID, *update.Nickname, s.filterNickname,
		); ok {
			update.Nickname = &masked
		}
	}
	if update.Abstract != nil {
		if masked, ok := s.filterOneProfileText(
			ctx, userID, *update.Abstract, s.filterAbstract,
		); ok {
			update.Abstract = &masked
		}
	}
	return update
}

// filterNickname 是昵称面的过滤调用；未装配时返回 false（调用方保留原文）。
func (s *Service) filterNickname(ctx context.Context, in FilterText) (FilteredText, bool, error) {
	if s.nicknameFilter == nil {
		return FilteredText{}, false, nil
	}
	got, err := s.nicknameFilter.FilterNicknameText(ctx, in)
	return got, err == nil, err
}

// filterAbstract 是简介面的过滤调用；未装配时返回 false（调用方保留原文）。
func (s *Service) filterAbstract(ctx context.Context, in FilterText) (FilteredText, bool, error) {
	if s.abstractFilter == nil {
		return FilteredText{}, false, nil
	}
	got, err := s.abstractFilter.FilterAbstractText(ctx, in)
	return got, err == nil, err
}

// filterOneProfileText 过滤一段资料文本；返回 ok=false 表示"保持原文"。
func (s *Service) filterOneProfileText(
	ctx context.Context,
	userID domain.UserID,
	text string,
	apply func(context.Context, FilterText) (FilteredText, bool, error),
) (string, bool) {
	if apply == nil || strings.TrimSpace(text) == "" {
		return text, true
	}
	got, wired, err := apply(ctx, FilterText{
		UserID:    userID,
		ContentID: ctype.NewID(userID.Uint64()),
		Text:      text,
	})
	if !wired {
		return text, true
	}
	if err != nil {
		s.logError("过滤用户资料失败，按原文保存: user=%s err=%v", userID.String(), err)
		return text, false
	}
	if got.Masked == "" {
		return text, false
	}
	return got.Masked, true
}
