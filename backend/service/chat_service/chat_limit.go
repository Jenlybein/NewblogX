package chat_service

import (
	"errors"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum/relationship_enum"
	"myblogx/service/follow_service"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_chat"
	"time"

	"gorm.io/gorm"
)

// ChatSendReservation 表示一次聊天发送前置检查成功后的 Redis 预占结果。
// 发送链路在消息真正入库成功后调用 Commit，在落库失败时调用 Rollback。
type ChatSendReservation struct {
	minuteReservation *redis_chat.MinuteReservation
	weekReservation   *redis_chat.WeekQuotaReservation
	resetWeekQuota    bool
	senderID          ctype.ID
	receiverID        ctype.ID
	now               time.Time
	redisDeps         redis_service.Deps
}

// Commit 在消息真正发送成功后提交本次预占结果。
// 对单向关系来说，提交时会顺手清空对向自然周配额，表示“对方已回复”。
func (r *ChatSendReservation) Commit() error {
	if r == nil || !r.resetWeekQuota {
		return nil
	}
	return redis_chat.ResetChatWeekQuota(r.redisDeps, r.receiverID, r.senderID, r.now)
}

// Rollback 在消息最终未落库时撤销本次 Redis 预占，避免失败请求占用额度。
func (r *ChatSendReservation) Rollback() error {
	if r == nil {
		return nil
	}
	if r.weekReservation != nil {
		if err := r.weekReservation.Release(r.redisDeps); err != nil {
			return err
		}
	}
	if r.minuteReservation != nil {
		if err := r.minuteReservation.Release(r.redisDeps); err != nil {
			return err
		}
	}
	return nil
}

// CheckAndReserveChatSend 统一处理聊天发送前的权限校验与 Redis 额度预占。
func CheckAndReserveChatSend(db *gorm.DB, redisDeps redis_service.Deps, senderID ctype.ID, receiver *models.UserModel) (*ChatSendReservation, error) {
	// 关系权限校验
	// 陌生人：如果用户设置接收陌生人消息才允许发送，每周只允许发一条消息
	// 好友：好友之间可以互发消息
	// 粉丝：若关注者未回复，粉丝每周可以向关注者发送 3 条消息
	// 关注者：若粉丝未回复，关注者每周可以向粉丝发送 3 条消息
	relation := relationship_enum.RelationStranger
	if senderID != receiver.ID {
		relation = follow_service.CalUserRelationship(db, senderID, receiver.ID)
	}

	switch relation {
	case relationship_enum.RelationStranger:
		if senderID != receiver.ID {
			if receiver.UserConfModel == nil || !receiver.UserConfModel.StrangerChatEnabled {
				return nil, errors.New("对方未开启陌生人私信")
			}
		}
	case relationship_enum.RelationFriend:
	case relationship_enum.RelationFans, relationship_enum.RelationFollowed:
	default:
		return nil, errors.New("当前关系不支持发送消息")
	}

	// 分钟级滑动窗口限流
	// 每分钟单个会话限制 30 条，跨会话限制 60 条
	now := time.Now()
	sessionID := buildSessionID(senderID, receiver.ID)
	minuteReservation, limitedBy, err := redis_chat.ReserveChatMinuteRate(redisDeps, senderID, sessionID, now)
	if err != nil {
		return nil, err
	}
	if minuteReservation == nil {
		switch limitedBy {
		case "session":
			return nil, errors.New("当前会话发送过于频繁，请稍后再试")
		default:
			return nil, errors.New("发送过于频繁，请稍后再试")
		}
	}

	// 单向关系(仅关注，仅粉丝，非好友)的每周聊天次数限制
	reservation := &ChatSendReservation{
		minuteReservation: minuteReservation,
		senderID:          senderID,
		receiverID:        receiver.ID,
		now:               now,
		redisDeps:         redisDeps,
	}

	// 给自己发消息和好友关系不走自然周配额限制。
	if senderID == receiver.ID || relation == relationship_enum.RelationFriend {
		return reservation, nil
	}

	// 陌生人：对方开启陌生人私信后，每自然周只允许发 1 条消息。
	if relation == relationship_enum.RelationStranger {
		weekReservation, allowed, err := redis_chat.ReserveChatWeekQuota(redisDeps, senderID, receiver.ID, 1, now)
		if err != nil {
			_ = reservation.Rollback()
			return nil, err
		}
		if !allowed {
			_ = reservation.Rollback()
			return nil, errors.New("本周只允许向陌生人发送 1 条消息")
		}

		reservation.weekReservation = weekReservation
		return reservation, nil
	}

	// 单向关系(仅关注，仅粉丝，非好友)的每周聊天次数限制。
	weekReservation, allowed, err := redis_chat.ReserveChatWeekQuota(redisDeps, senderID, receiver.ID, 3, now)
	if err != nil {
		_ = reservation.Rollback()
		return nil, err
	}
	if !allowed {
		_ = reservation.Rollback()
		return nil, errors.New("本周可发送消息次数已达上限，对方回复后才可以继续发送消息")
	}

	reservation.weekReservation = weekReservation
	reservation.resetWeekQuota = true
	return reservation, nil
}
