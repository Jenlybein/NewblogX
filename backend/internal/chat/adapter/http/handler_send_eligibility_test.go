package http_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/chat/app"
	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/res"
)

// 这一组用例从 **HTTP 发送接口**观察 #280 的发送资格语义：
// 站点总闸、接收方个人开关、屏蔽 / 拉黑分级、按对限流、图片来源在同一次
// 判定里判完；任何一项依赖没装配或查询异常，都必须是"拒绝发送"，
// 而不是跳过这一项静默放行。

type permitBlocks struct{}

func (permitBlocks) Level(context.Context, ctype.ID, ctype.ID) (domain.BlockLevel, error) {
	return domain.BlockLevelNone, nil
}

type failingBlocks struct{ err error }

func (f failingBlocks) Level(context.Context, ctype.ID, ctype.ID) (domain.BlockLevel, error) {
	return domain.BlockLevelNone, f.err
}

type permitLimiter struct{}

func (permitLimiter) Reserve(context.Context, ctype.ID, ctype.ID, int8) (domain.Reservation, error) {
	return noReservation{}, nil
}

type noReservation struct{}

func (noReservation) Commit() error   { return nil }
func (noReservation) Rollback() error { return nil }

type failingLimiter struct{ err error }

func (f failingLimiter) Reserve(context.Context, ctype.ID, ctype.ID, int8) (domain.Reservation, error) {
	return nil, f.err
}

type allowImage struct{}

func (allowImage) IsOwnedImageURL(context.Context, string) (bool, error) { return true, nil }

type failingStranger struct{ err error }

func (f failingStranger) CheckStranger(context.Context, ctype.ID) (domain.StrangerVerdict, error) {
	return domain.StrangerAllowed, f.err
}

// eligibilityInputs 只描述本组用例关心的协作者；其余用"明确允许"的替身。
type eligibilityInputs struct {
	relations domain.RelationReader
	stranger  domain.StrangerPolicy
	limiter   domain.Limiter
	blocks    domain.BlockPolicy
	image     domain.ImageURLVerifier
}

func newEligibilityService(d eligibilityInputs) (*app.Service, *fakeRepo) {
	repo := &fakeRepo{}
	svc := app.New(repo, repo, d.relations, fakeUsers{}, d.stranger, d.limiter, d.image, d.blocks, nil)
	return svc, repo
}

// TestSendEligibility_依赖缺失时一律拒绝发送 五项资格依赖任一没装配，
// 发送都必须被拒绝：跳过一项等于把那一项的风控静默关掉。
func TestSendEligibility_依赖缺失时一律拒绝发送(t *testing.T) {
	cases := []struct {
		name string
		deps eligibilityInputs
	}{
		{
			name: "屏蔽与拉黑策略未装配",
			deps: eligibilityInputs{
				relations: fakeRelations{relation: 4},
				stranger:  fakeStranger{verdict: domain.StrangerAllowed},
				limiter:   permitLimiter{},
				blocks:    nil,
				image:     allowImage{},
			},
		},
		{
			name: "关注关系未装配",
			deps: eligibilityInputs{
				relations: nil,
				stranger:  fakeStranger{verdict: domain.StrangerAllowed},
				limiter:   permitLimiter{},
				blocks:    permitBlocks{},
				image:     allowImage{},
			},
		},
		{
			name: "陌生人策略未装配",
			deps: eligibilityInputs{
				relations: fakeRelations{relation: 1},
				stranger:  nil,
				limiter:   permitLimiter{},
				blocks:    permitBlocks{},
				image:     allowImage{},
			},
		},
		{
			name: "按对限流器未装配",
			deps: eligibilityInputs{
				relations: fakeRelations{relation: 4},
				stranger:  fakeStranger{verdict: domain.StrangerAllowed},
				limiter:   nil,
				blocks:    permitBlocks{},
				image:     allowImage{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newEligibilityService(tc.deps)
			engine := newEngine(svc)

			got := postMessage(t, engine, 2, "你好")
			if got.Code == res.SuccessCode {
				t.Fatalf("资格依赖缺失时必须拒绝发送，实际成功回执 code=%d msg=%q", got.Code, got.Msg)
			}
			if len(repo.sent) != 0 {
				t.Fatalf("被拒绝的消息不能落库，实际写入 %d 条", len(repo.sent))
			}
		})
	}
}

// TestSendEligibility_依赖异常时一律拒绝发送 依赖在但查询报错时，
// 必须原样中止发送，不能吞成"没有记录 / 放行"。
func TestSendEligibility_依赖异常时一律拒绝发送(t *testing.T) {
	boom := errors.New("资格依赖查询失败")
	cases := []struct {
		name string
		deps eligibilityInputs
	}{
		{
			name: "屏蔽与拉黑查询异常",
			deps: eligibilityInputs{
				relations: fakeRelations{relation: 4},
				stranger:  fakeStranger{verdict: domain.StrangerAllowed},
				limiter:   permitLimiter{},
				blocks:    failingBlocks{err: boom},
				image:     allowImage{},
			},
		},
		{
			name: "关注关系查询异常",
			deps: eligibilityInputs{
				relations: failingRelations{err: boom},
				stranger:  fakeStranger{verdict: domain.StrangerAllowed},
				limiter:   permitLimiter{},
				blocks:    permitBlocks{},
				image:     allowImage{},
			},
		},
		{
			name: "陌生人判定异常",
			deps: eligibilityInputs{
				relations: fakeRelations{relation: 1},
				stranger:  failingStranger{err: boom},
				limiter:   permitLimiter{},
				blocks:    permitBlocks{},
				image:     allowImage{},
			},
		},
		{
			name: "按对限流预占异常",
			deps: eligibilityInputs{
				relations: fakeRelations{relation: 4},
				stranger:  fakeStranger{verdict: domain.StrangerAllowed},
				limiter:   failingLimiter{err: boom},
				blocks:    permitBlocks{},
				image:     allowImage{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newEligibilityService(tc.deps)
			engine := newEngine(svc)

			got := postMessage(t, engine, 2, "你好")
			if got.Code == res.SuccessCode {
				t.Fatalf("资格依赖异常时必须拒绝发送，实际成功回执 code=%d msg=%q", got.Code, got.Msg)
			}
			if len(repo.sent) != 0 {
				t.Fatalf("被拒绝的消息不能落库，实际写入 %d 条", len(repo.sent))
			}
		})
	}
}

type failingRelations struct{ err error }

func (f failingRelations) Batch(context.Context, ctype.ID, []ctype.ID) (map[ctype.ID]int8, error) {
	return nil, f.err
}
