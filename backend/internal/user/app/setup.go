package app

import (
	"context"

	"myblogx/internal/apperr"
	"myblogx/internal/user/domain"
)

// SetupStatus 是首次初始化的状态。
type SetupStatus struct {
	// NeedsAdmin 为真表示库里还没有任何管理员，初始化入口开放。
	NeedsAdmin bool
}

// setupAdminPageSize 用于"有没有管理员"这一次查询：
// 只要知道总数，不需要真的取回列表。
const setupAdminPageSize = 1

// AdminExists 判断库里是否已经有管理员。
//
// 只看角色，不看状态：哪怕唯一的管理员被停用了，初始化入口也必须关闭 ——
// 否则任何人都能在站点被停用管理员后重新抢注一个管理员账号。
func (s *Service) AdminExists(ctx context.Context) (bool, error) {
	role := domain.RoleAdmin
	page, err := s.users.List(ctx, domain.Query{
		Role: &role,
		Page: 1,
		Size: setupAdminPageSize,
	})
	if err != nil {
		return false, err
	}
	return page.Total > 0, nil
}

// SetupStatus 返回首次初始化状态。
func (s *Service) SetupStatus(ctx context.Context) (SetupStatus, error) {
	exists, err := s.AdminExists(ctx)
	if err != nil {
		return SetupStatus{}, err
	}
	return SetupStatus{NeedsAdmin: !exists}, nil
}

// SetupFirstAdmin 创建站点第一个管理员。
//
// 这是唯一一条不经过鉴权就能建账号的路径，所以闸门必须写在这里而不是
// handler 里：将来多一个入口也不会漏。闸门是「库里还没有管理员」——
// 一旦有了，这个接口就永久返回 setup.already_done。
//
// 与 Create 的区别只有两点：跳过权限校验（此刻还没有人可以校验），
// 以及把角色固定成管理员。
func (s *Service) SetupFirstAdmin(ctx context.Context, in CreateInput) (domain.User, error) {
	exists, err := s.AdminExists(ctx)
	if err != nil {
		return domain.User{}, err
	}
	if exists {
		return domain.User{}, apperr.New(apperr.CodeSetupAlreadyDone)
	}

	in.Role = domain.RoleAdmin
	in.Status = domain.StatusActive
	return s.create(ctx, in)
}
