package message_service_test

import (
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum/message_enum"
	"myblogx/service/message_service"
	"myblogx/test/testutil"
	"testing"
)

func setupMessageServiceEnv(t *testing.T) {
	t.Helper()
	_ = testutil.SetupSQLite(t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.ArticleMessageModel{},
	)
}

func createMessageUser(t *testing.T, username, nickname, avatar string) models.UserModel {
	t.Helper()

	user := models.UserModel{
		Username: username,
		Password: "test-password",
		Nickname: nickname,
		Avatar:   avatar,
	}
	if err := testutil.DB().Create(&user).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	return user
}

func loadArticleMessages(t *testing.T) []models.ArticleMessageModel {
	t.Helper()

	var list []models.ArticleMessageModel
	if err := testutil.DB().Order("id asc").Find(&list).Error; err != nil {
		t.Fatalf("查询消息失败: %v", err)
	}
	return list
}

func TestInsertCommentAndReplyMessage(t *testing.T) {
	setupMessageServiceEnv(t)

	actionUser := createMessageUser(t, "action_user", "Alice", "/avatar/alice.png")
	receiver := createMessageUser(t, "receiver_user", "Bob", "/avatar/bob.png")

	message_service.InsertCommentMessage(testutil.DB(), testutil.Logger(), message_service.ArticleCommentMessage{
		CommentID:    11,
		Content:      "评论内容",
		ReceiverID:   receiver.ID,
		ActionUserID: actionUser.ID,
		ArticleID:    21,
		ArticleTitle: "文章 A",
	})
	message_service.InsertReplyMessage(testutil.DB(), testutil.Logger(), message_service.ArticleReplyMessage{
		CommentID:    12,
		Content:      "回复内容",
		ReceiverID:   receiver.ID,
		ActionUserID: actionUser.ID,
		ArticleID:    22,
		ArticleTitle: "文章 B",
	})

	list := loadArticleMessages(t)
	if len(list) != 2 {
		t.Fatalf("消息数量错误: got=%d", len(list))
	}

	if list[0].Type != message_enum.CommentArticleType {
		t.Fatalf("评论消息类型错误: got=%v", list[0].Type)
	}
	if list[0].ReceiverID != receiver.ID || list[0].CommentID != 11 || list[0].ArticleID != 21 {
		t.Fatalf("评论消息业务字段错误: %+v", list[0])
	}
	if list[0].ActionUserID == nil || *list[0].ActionUserID != actionUser.ID {
		t.Fatalf("评论消息操作用户错误: %+v", list[0])
	}
	if list[0].ActionUserNickname == nil || *list[0].ActionUserNickname != "Alice" {
		t.Fatalf("评论消息昵称错误: %+v", list[0])
	}
	if list[0].ActionUserAvatar == nil || *list[0].ActionUserAvatar != "/avatar/alice.png" {
		t.Fatalf("评论消息头像错误: %+v", list[0])
	}
	if list[0].Content != "评论内容" || list[0].ArticleTitle != "文章 A" {
		t.Fatalf("评论消息内容错误: %+v", list[0])
	}

	if list[1].Type != message_enum.CommentReplyType {
		t.Fatalf("回复消息类型错误: got=%v", list[1].Type)
	}
	if list[1].ReceiverID != receiver.ID || list[1].CommentID != 12 || list[1].ArticleID != 22 {
		t.Fatalf("回复消息业务字段错误: %+v", list[1])
	}
	if list[1].ActionUserNickname == nil || *list[1].ActionUserNickname != "Alice" {
		t.Fatalf("回复消息昵称错误: %+v", list[1])
	}
	if list[1].ActionUserAvatar == nil || *list[1].ActionUserAvatar != "/avatar/alice.png" {
		t.Fatalf("回复消息头像错误: %+v", list[1])
	}
	if list[1].Content != "回复内容" || list[1].ArticleTitle != "文章 B" {
		t.Fatalf("回复消息内容错误: %+v", list[1])
	}
}

func TestInsertDiggAndFavorMessagesDeduplicate(t *testing.T) {
	cases := []struct {
		name        string
		wantType    message_enum.Type
		wantArticle ctype.ID
		wantComment ctype.ID
		wantContent string
		insertTwice func(receiverID, actionUserID ctype.ID)
	}{
		{
			name:        "article digg",
			wantType:    message_enum.DiggArticleType,
			wantArticle: 31,
			insertTwice: func(receiverID, actionUserID ctype.ID) {
				content := message_service.ArticleDiggMessage{
					ReceiverID:   receiverID,
					ActionUserID: actionUserID,
					ArticleID:    31,
					ArticleTitle: "被点赞文章",
				}
				message_service.InsertArticleDiggMessage(testutil.DB(), testutil.Logger(), content)
				message_service.InsertArticleDiggMessage(testutil.DB(), testutil.Logger(), content)
			},
		},
		{
			name:        "comment digg",
			wantType:    message_enum.DiggCommentType,
			wantArticle: 32,
			wantComment: 41,
			wantContent: "评论被点赞",
			insertTwice: func(receiverID, actionUserID ctype.ID) {
				content := message_service.CommentDiggMessage{
					CommentID:    41,
					Content:      "评论被点赞",
					ReceiverID:   receiverID,
					ActionUserID: actionUserID,
					ArticleID:    32,
					ArticleTitle: "评论所属文章",
				}
				message_service.InsertCommentDiggMessage(testutil.DB(), testutil.Logger(), content)
				message_service.InsertCommentDiggMessage(testutil.DB(), testutil.Logger(), content)
			},
		},
		{
			name:        "article favor",
			wantType:    message_enum.FavorArticleType,
			wantArticle: 33,
			insertTwice: func(receiverID, actionUserID ctype.ID) {
				content := message_service.ArticleFavorMessage{
					ReceiverID:   receiverID,
					ActionUserID: actionUserID,
					ArticleID:    33,
					ArticleTitle: "被收藏文章",
				}
				message_service.InsertArticleFavorMessage(testutil.DB(), testutil.Logger(), content)
				message_service.InsertArticleFavorMessage(testutil.DB(), testutil.Logger(), content)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupMessageServiceEnv(t)

			actionUser := createMessageUser(t, "action_"+tc.name, "Tester", "/avatar/tester.png")
			receiver := createMessageUser(t, "receiver_"+tc.name, "Receiver", "/avatar/receiver.png")

			tc.insertTwice(receiver.ID, actionUser.ID)

			list := loadArticleMessages(t)
			if len(list) != 1 {
				t.Fatalf("去重失败，消息数量错误: got=%d", len(list))
			}

			got := list[0]
			if got.Type != tc.wantType {
				t.Fatalf("消息类型错误: got=%v want=%v", got.Type, tc.wantType)
			}
			if got.ReceiverID != receiver.ID {
				t.Fatalf("接收者错误: got=%d want=%d", got.ReceiverID, receiver.ID)
			}
			if got.ActionUserID == nil || *got.ActionUserID != actionUser.ID {
				t.Fatalf("操作用户错误: %+v", got)
			}
			if got.ActionUserNickname == nil || *got.ActionUserNickname != "Tester" {
				t.Fatalf("操作用户昵称错误: %+v", got)
			}
			if got.ActionUserAvatar == nil || *got.ActionUserAvatar != "/avatar/tester.png" {
				t.Fatalf("操作用户头像错误: %+v", got)
			}
			if got.ArticleID != tc.wantArticle || got.CommentID != tc.wantComment {
				t.Fatalf("业务主键错误: %+v", got)
			}
			if got.Content != tc.wantContent {
				t.Fatalf("消息内容错误: got=%q want=%q", got.Content, tc.wantContent)
			}
		})
	}
}

func TestInsertCommentMessageWithoutActionUserInfo(t *testing.T) {
	setupMessageServiceEnv(t)

	receiver := createMessageUser(t, "receiver_only", "Receiver", "/avatar/receiver.png")

	message_service.InsertCommentMessage(testutil.DB(), testutil.Logger(), message_service.ArticleCommentMessage{
		CommentID:    51,
		Content:      "缺失用户信息",
		ReceiverID:   receiver.ID,
		ActionUserID: 9999,
		ArticleID:    61,
		ArticleTitle: "文章 C",
	})

	list := loadArticleMessages(t)
	if len(list) != 1 {
		t.Fatalf("消息数量错误: got=%d", len(list))
	}

	got := list[0]
	if got.Type != message_enum.CommentArticleType {
		t.Fatalf("消息类型错误: got=%v", got.Type)
	}
	if got.ActionUserID == nil || *got.ActionUserID != 9999 {
		t.Fatalf("操作用户 ID 错误: %+v", got)
	}
	if got.ActionUserNickname == nil || *got.ActionUserNickname != "" {
		t.Fatalf("缺失用户时昵称应为空字符串: %+v", got)
	}
	if got.ActionUserAvatar == nil || *got.ActionUserAvatar != "" {
		t.Fatalf("缺失用户时头像应为空字符串: %+v", got)
	}
}

func TestInsertSystemMessage(t *testing.T) {
	t.Run("带操作用户时自动补全用户信息", func(t *testing.T) {
		setupMessageServiceEnv(t)

		actionUser := createMessageUser(t, "system_action", "SystemAlice", "/avatar/system-alice.png")
		receiver := createMessageUser(t, "system_receiver", "Receiver", "/avatar/receiver.png")

		message_service.InsertSystemMessage(testutil.DB(), testutil.Logger(), message_service.SystemMessage{
			ReceiverID:   receiver.ID,
			ActionUserID: &actionUser.ID,
			Content:      "系统通知内容",
			LinkTitle:    "查看详情",
			LinkHerf:     "/articles/71",
		})

		list := loadArticleMessages(t)
		if len(list) != 1 {
			t.Fatalf("消息数量错误: got=%d", len(list))
		}

		got := list[0]
		if got.Type != message_enum.SystemType {
			t.Fatalf("系统消息类型错误: got=%v", got.Type)
		}
		if got.ActionUserID == nil || *got.ActionUserID != actionUser.ID {
			t.Fatalf("系统消息操作用户错误: %+v", got)
		}
		if got.ActionUserNickname == nil || *got.ActionUserNickname != "SystemAlice" {
			t.Fatalf("系统消息昵称错误: %+v", got)
		}
		if got.ActionUserAvatar == nil || *got.ActionUserAvatar != "/avatar/system-alice.png" {
			t.Fatalf("系统消息头像错误: %+v", got)
		}
		if got.Content != "系统通知内容" {
			t.Fatalf("系统消息内容错误: %+v", got)
		}
		if got.LinkTitle != "查看详情" || got.LinkHerf != "/articles/71" {
			t.Fatalf("系统消息链接字段错误: %+v", got)
		}
	})

	t.Run("无操作用户时允许发送纯系统通知", func(t *testing.T) {
		setupMessageServiceEnv(t)

		receiver := createMessageUser(t, "system_receiver_only", "Receiver", "/avatar/receiver.png")

		message_service.InsertSystemMessage(testutil.DB(), testutil.Logger(), message_service.SystemMessage{
			ReceiverID: receiver.ID,
			Content:    "纯系统消息",
			LinkTitle:  "查看公告",
			LinkHerf:   "/notice/1",
		})

		list := loadArticleMessages(t)
		if len(list) != 1 {
			t.Fatalf("消息数量错误: got=%d", len(list))
		}

		got := list[0]
		if got.Type != message_enum.SystemType {
			t.Fatalf("系统消息类型错误: got=%v", got.Type)
		}
		if got.ActionUserID != nil || got.ActionUserNickname != nil || got.ActionUserAvatar != nil {
			t.Fatalf("纯系统消息不应带操作用户信息: %+v", got)
		}
		if got.Content != "纯系统消息" || got.LinkTitle != "查看公告" || got.LinkHerf != "/notice/1" {
			t.Fatalf("纯系统消息字段错误: %+v", got)
		}
	})

	t.Run("缺少具体接收者时不创建消息", func(t *testing.T) {
		setupMessageServiceEnv(t)

		message_service.InsertSystemMessage(testutil.DB(), testutil.Logger(), message_service.SystemMessage{
			ReceiverID: 0,
			Content:    "无效系统消息",
		})

		list := loadArticleMessages(t)
		if len(list) != 0 {
			t.Fatalf("缺少接收者时不应创建消息: %+v", list)
		}
	})
}
