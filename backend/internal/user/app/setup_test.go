package app_test

import (
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/user/app"
	"myblogx/internal/user/apptest"
	"myblogx/internal/user/domain"
)

// 首次初始化的闸门是"库里还没有管理员"。
// 这组用例守的不是"能不能建账号"，而是"这个公开接口会不会变成后门"。

func TestSetupStatus_空库时要求初始化(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)

	status, err := r.Service().SetupStatus(testCtx())
	if err != nil {
		t.Fatalf("查询初始化状态失败: %v", err)
	}
	if !status.NeedsAdmin {
		t.Fatal("库里没有管理员时应要求初始化")
	}
}

func TestSetupStatus_已有管理员时关闭(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	r.Users().Seed(domain.User{
		Username: "boss", Nickname: "站长",
		Status: domain.StatusActive, Role: domain.RoleAdmin,
	})

	status, err := r.Service().SetupStatus(testCtx())
	if err != nil {
		t.Fatalf("查询初始化状态失败: %v", err)
	}
	if status.NeedsAdmin {
		t.Fatal("已有管理员时不该再要求初始化")
	}
}

func TestSetupStatus_只有普通用户时仍要求初始化(t *testing.T) {
	// 站点可能先有注册用户、后才有管理员（比如改过闸门顺序）。
	// 判断依据是"有没有管理员"，不是"有没有用户"。
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	r.Users().Seed(domain.User{
		Username: "bob", Nickname: "路人",
		Status: domain.StatusActive, Role: domain.RoleUser,
	})

	status, err := r.Service().SetupStatus(testCtx())
	if err != nil {
		t.Fatalf("查询初始化状态失败: %v", err)
	}
	if !status.NeedsAdmin {
		t.Fatal("只有普通用户时仍应要求初始化")
	}
}

func TestSetupFirstAdmin_建出来的账号一定是管理员(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)

	// 故意传普通用户角色与停用状态：初始化接口不接受调用方指定这两项。
	u, err := r.Service().SetupFirstAdmin(testCtx(), app.CreateInput{
		Username: "root",
		Nickname: "站长",
		Password: "Passw0rd!",
		Role:     domain.RoleUser,
		Status:   domain.StatusDisabled,
	})
	if err != nil {
		t.Fatalf("创建第一个管理员失败: %v", err)
	}
	if u.Role != domain.RoleAdmin {
		t.Fatalf("角色应被强制为管理员，实际 %v", u.Role)
	}
	if u.Status != domain.StatusActive {
		t.Fatalf("状态应被强制为启用，实际 %v", u.Status)
	}

	status, err := r.Service().SetupStatus(testCtx())
	if err != nil {
		t.Fatalf("查询初始化状态失败: %v", err)
	}
	if status.NeedsAdmin {
		t.Fatal("创建后初始化入口应关闭")
	}
}

func TestSetupFirstAdmin_已有管理员时永久关闭(t *testing.T) {
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	r.Users().Seed(domain.User{
		Username: "boss", Nickname: "站长",
		Status: domain.StatusActive, Role: domain.RoleAdmin,
	})

	_, err := r.Service().SetupFirstAdmin(testCtx(), app.CreateInput{
		Username: "intruder",
		Nickname: "闯入者",
		Password: "Passw0rd!",
	})
	assertCode(t, err, apperr.CodeSetupAlreadyDone)
}

func TestSetupFirstAdmin_管理员被停用也不算关闭(t *testing.T) {
	// 只看角色不看状态：若按"有没有启用的管理员"判断，
	// 有人把唯一的管理员停用之后，这个公开接口就又开了一条缝。
	t.Parallel()
	r := apptest.NewRepos(baseTime)
	r.Users().Seed(domain.User{
		Username: "boss", Nickname: "站长",
		Status: domain.StatusDisabled, Role: domain.RoleAdmin,
	})

	_, err := r.Service().SetupFirstAdmin(testCtx(), app.CreateInput{
		Username: "intruder",
		Nickname: "闯入者",
		Password: "Passw0rd!",
	})
	assertCode(t, err, apperr.CodeSetupAlreadyDone)
}

func TestSetupFirstAdmin_复用创建用户的全套校验(t *testing.T) {
	// 初始化路径不能成为"绕开校验"的口子：用户名格式、重名、
	// 空密码这些都得和正常创建一样被拦住。
	t.Parallel()

	t.Run("用户名为空", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		_, err := r.Service().SetupFirstAdmin(testCtx(), app.CreateInput{
			Nickname: "站长", Password: "Passw0rd!",
		})
		if err == nil {
			t.Fatal("空用户名应被拒绝")
		}
	})

	t.Run("密码为空", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		_, err := r.Service().SetupFirstAdmin(testCtx(), app.CreateInput{
			Username: "root", Nickname: "站长",
		})
		assertCode(t, err, apperr.CodeUserPasswordRequired)
	})

	t.Run("用户名已被占用", func(t *testing.T) {
		t.Parallel()
		r := apptest.NewRepos(baseTime)
		r.Users().Seed(domain.User{
			Username: "root", Nickname: "旧账号", Status: domain.StatusActive, Role: domain.RoleUser,
		})
		_, err := r.Service().SetupFirstAdmin(testCtx(), app.CreateInput{
			Username: "root", Nickname: "站长", Password: "Passw0rd!",
		})
		assertCode(t, err, apperr.CodeUserUsernameTaken)
	})
}
