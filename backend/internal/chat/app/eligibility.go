package app

import (
	"context"
	"encoding/json"

	"myblogx/internal/apperr"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
)

// sendGuard 是私信发送资格的唯一判定点（#280）。
//
// 站点总闸 / 接收方个人开关（合并在 StrangerPolicy 结论里）、屏蔽与拉黑分级、
// 按对限流、图片来源这五项在这里**一次判完**：要么带回完整的发送计划
// （展示快照、关系级别、已预占的额度、归一化后的正文），要么直接拒绝。
//
// 失败关闭：任何一项依赖没装配，或查询报错，都返回拒绝，不跳过这一项。
// 之前每个依赖各自有一个 nil 放行分支，等于把风控交给"装配时有没有传"。
type sendGuard struct {
	read       domain.Query
	blocks     domain.BlockPolicy
	relations  domain.RelationReader
	users      domain.UserDirectory
	stranger   domain.StrangerPolicy
	limiter    domain.Limiter
	imageCheck domain.ImageURLVerifier
}

// sendPlan 是一次通过资格判定的发送计划。
//
// reservation 由发送者负责收尾：落库失败 / 幂等命中时回滚，正常落库后提交。
type sendPlan struct {
	senderID    ctype.ID
	receiverID  ctype.ID
	sender      domain.Display
	receiver    domain.Display
	relation    int8
	reservation domain.Reservation
	content     string
}

// check 判完发送资格。返回的 content 是归一化后的正文（图片消息已包成 JSON）。
//
// 四层判定各是一个私有方法，顺序即优先级（后面的层看不到前面层否决的请求）：
// validate（依赖 + 图片）→ rejectBlocked（拉黑）→ rejectStranger（陌生人）→ reserve（限流）。
// 这里只负责按顺序串起来并把结论拼成发送计划。
func (g *sendGuard) check(ctx context.Context, senderID, receiverID ctype.ID, msgType domain.MsgType, content string) (sendPlan, error) {
	normalized, err := g.validate(ctx, msgType, content)
	if err != nil {
		return sendPlan{}, err
	}
	if err := g.rejectBlocked(ctx, senderID, receiverID); err != nil {
		return sendPlan{}, err
	}
	sender, receiver, relation, err := g.rejectStranger(ctx, senderID, receiverID)
	if err != nil {
		return sendPlan{}, err
	}
	reservation, err := g.reserve(ctx, senderID, receiverID, relation)
	if err != nil {
		return sendPlan{}, err
	}

	return sendPlan{
		senderID:    senderID,
		receiverID:  receiverID,
		sender:      sender,
		receiver:    receiver,
		relation:    relation,
		reservation: reservation,
		content:     normalized,
	}, nil
}

// validate 是第 1 层：资格依赖必须全部装配，图片消息必须来自本站上传产物。
//
// 五项资格依赖缺一不可：缺项意味着对应的风控根本没上场，不能放行。
func (g *sendGuard) validate(ctx context.Context, msgType domain.MsgType, content string) (string, error) {
	if g.blocks == nil || g.relations == nil || g.stranger == nil || g.limiter == nil {
		return "", apperr.New(apperr.CodeChatSendFailed)
	}
	return g.verifyImage(ctx, msgType, content)
}

// rejectBlocked 是第 2 层：屏蔽 / 拉黑（ADR-0026）在陌生人判定之前。
//
// 这是"我"明确表达过的意愿，优先级高于任何站点级 / 个人级陌生人开关：
//   - 我屏蔽 / 拉黑了对方：我不该再能给他发（单向自律，避免手滑继续骚扰）；
//   - 对方屏蔽 / 拉黑了我：他发不进来，我也不该能发过去。
func (g *sendGuard) rejectBlocked(ctx context.Context, senderID, receiverID ctype.ID) error {
	outgoing, err := g.blocks.Level(ctx, senderID, receiverID)
	if err != nil {
		return err
	}
	incoming, err := g.blocks.Level(ctx, receiverID, senderID)
	if err != nil {
		return err
	}
	if outgoing != domain.BlockLevelNone || incoming != domain.BlockLevelNone {
		return apperr.New(apperr.CodeChatBlocked)
	}
	return nil
}

// rejectStranger 是第 3 层：陌生人判定。
//
// 展示快照与关系级别是这一层的输入（"确实没有关系"才可能是陌生人），所以和
// 判定一起取并带回给发送计划 —— 拆成"先取数再判定"只会让取数在没有判定时
// 也被调一次。
//
// 陌生人私信是**两级**策略（#243）：站点总闸 AND 接收方个人开关。
// 两级取值在装配层的 StrangerPolicy 里合成，用例层只消费结论；
// 必须传接收方 ID，个人开关才有接线点。
//
// #247 起结论带原因：两种拒绝在读者侧对应不同的正确动作 ——
// 站点级找管理员，接收方级提示"对方不接受陌生人私信"。
func (g *sendGuard) rejectStranger(ctx context.Context, senderID, receiverID ctype.ID) (sender, receiver domain.Display, relation int8, err error) {
	displays, err := g.users.Displays(ctx, []ctype.ID{senderID, receiverID})
	if err != nil {
		return domain.Display{}, domain.Display{}, 0, err
	}
	receiver, ok := displays[receiverID]
	if !ok {
		return domain.Display{}, domain.Display{}, 0, apperr.New(apperr.CodeChatUserNotFound)
	}
	sender = displays[senderID]

	relation = int8(1)
	if senderID != receiverID {
		got, err := g.relations.Batch(ctx, senderID, []ctype.ID{receiverID})
		if err != nil {
			return domain.Display{}, domain.Display{}, 0, err
		}
		if rel, ok := got[receiverID]; ok {
			relation = rel
		}
	}
	// 陌生人判定只对"确实没有关系"的双方做：自己、已有关注关系、
	// 或已存在会话的双方都不参与（#243）。
	stranger := senderID != receiverID && relation == 1
	if stranger {
		_, err := g.read.FindSession(ctx, domain.BuildSessionID(senderID, receiverID), senderID, true)
		switch {
		case err == nil:
			// 已经聊过：不再是陌生人，个人开关不该半路切断既有会话。
			stranger = false
		case apperr.IsCode(err, apperr.CodeChatSessionNotFound):
			// 没有会话：继续按陌生人判定。
		default:
			return domain.Display{}, domain.Display{}, 0, err
		}
	}
	if stranger {
		verdict, err := g.stranger.CheckStranger(ctx, receiverID)
		if err != nil {
			return domain.Display{}, domain.Display{}, 0, err
		}
		switch verdict {
		case domain.StrangerAllowed:
		case domain.StrangerBlockedByRecipient:
			return domain.Display{}, domain.Display{}, 0, apperr.New(apperr.CodeChatRecipientStrangerOff)
		default:
			// 站点级拒绝（以及未知结论的失败关闭）沿用既有错误码。
			return domain.Display{}, domain.Display{}, 0, apperr.New(apperr.CodeChatStrangerOff)
		}
	}
	return sender, receiver, relation, nil
}

// reserve 是第 4 层：按对限流。额度预占是发送计划的一部分，由发送者收尾。
func (g *sendGuard) reserve(ctx context.Context, senderID, receiverID ctype.ID, relation int8) (domain.Reservation, error) {
	return g.limiter.Reserve(ctx, senderID, receiverID, relation)
}

// verifyImage 校验图片私信必须来自本站上传产物，并保持既有 JSON 正文形状。
//
// 文本 / Markdown 完全不走这条校验。图片消息未装配校验端口时失败关闭：
// content 是通用字符串，不校验来源时任意外链都能借私信塞进来（票 267）。
func (g *sendGuard) verifyImage(ctx context.Context, msgType domain.MsgType, content string) (string, error) {
	if msgType != domain.TypeImage {
		return content, nil
	}
	if g.imageCheck == nil {
		return "", apperr.New(apperr.CodeChatImageNotOwned)
	}
	owned, err := g.imageCheck.IsOwnedImageURL(ctx, content)
	if err != nil {
		return "", err
	}
	if !owned {
		return "", apperr.New(apperr.CodeChatImageNotOwned)
	}
	payload, err := json.Marshal(map[string]string{"kind": "image", "image_url": content})
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// rollback 放弃本次额度预占（落库失败或幂等命中时调用）。
func (p sendPlan) rollback() {
	if p.reservation != nil {
		_ = p.reservation.Rollback()
	}
}

// commit 提交本次额度预占；提交失败只记日志，不影响已落库的消息。
func (p sendPlan) commit(logger domain.Logger) {
	if p.reservation == nil {
		return
	}
	if err := p.reservation.Commit(); err != nil && logger != nil {
		logger.Warnf("提交聊天发送额度失败: sender=%d receiver=%d err=%v", p.senderID, p.receiverID, err)
	}
}
