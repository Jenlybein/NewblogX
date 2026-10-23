package user_service_test

import (
	"myblogx/models"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_user"
	"myblogx/service/user_service"
	"myblogx/test/testutil"
	"testing"
)

func TestNewUserService(t *testing.T) {
	user := models.UserModel{
		Username: "u1",
	}
	s := user_service.NewUserService(user)
	if s == nil {
		t.Fatal("NewUserService 不应返回 nil")
	}
}

func TestNextAutoUsername(t *testing.T) {
	testutil.SetupMiniRedis(t)
	deps := redis_service.Deps{Client: testutil.Redis(), Logger: testutil.Logger()}

	username1, err := redis_user.NextAutoUsername(deps)
	if err != nil {
		t.Fatalf("首次生成用户名失败: %v", err)
	}
	if username1 != "100001" {
		t.Fatalf("首次用户名错误: got=%s want=100001", username1)
	}

	username2, err := redis_user.NextAutoUsername(deps)
	if err != nil {
		t.Fatalf("第二次生成用户名失败: %v", err)
	}
	if username2 != "100002" {
		t.Fatalf("第二次用户名错误: got=%s want=100002", username2)
	}
}
