package enum_test

import (
	"myblogx/models/enum"
	"testing"
)

func TestLogLevelString(t *testing.T) {
	if enum.LogInfoLevel.String() != "info" {
		t.Fatalf("LogInfoLevel.String 错误: %s", enum.LogInfoLevel.String())
	}
	if enum.LogWarnLevel.String() != "warn" {
		t.Fatalf("LogWarnLevel.String 错误: %s", enum.LogWarnLevel.String())
	}
	if enum.LogErrorLevel.String() != "error" {
		t.Fatalf("LogErrorLevel.String 错误: %s", enum.LogErrorLevel.String())
	}
}

func TestRoleString(t *testing.T) {
	if enum.RoleAdmin.String() == "" || enum.RoleUser.String() == "" || enum.RoleGuest.String() == "" {
		t.Fatal("角色字符串不应为空")
	}
	if enum.RoleType(100).String() == "" {
		t.Fatal("未知角色字符串不应为空")
	}
}
