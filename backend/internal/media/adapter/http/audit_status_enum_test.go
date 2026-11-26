package http

import (
	"reflect"
	"strings"
	"testing"

	"myblogx/internal/media/domain"
)

// auditStatusEnum 是契约里 `audit_status` 的取值集合（swag 的 enums 注解）。
//
// 客户端按它做分支（"审核中 vs 已拒绝"），所以它必须与 domain 的状态机一一对应：
// 少一个值，客户端就落到 default 分支、把具体结论说成"状态异常"；多一个值，
// 客户端会写出走不到的分支。
func auditStatusEnum() []string {
	return []string{"pending", "pass", "review", "block", "deleted", "orphaned", "unknown"}
}

// TestAuditStatusEnumMatchesDomain 钉住"契约枚举 = domain 状态集合"（比集合，不比顺序：
// 枚举在契约里的书写顺序对客户端没有意义，状态机里的顺序才有）。
func TestAuditStatusEnumMatchesDomain(t *testing.T) {
	t.Parallel()

	domainStatuses := []domain.Status{
		domain.StatusUnknown,
		domain.StatusPass,
		domain.StatusDeleted,
		domain.StatusOrphaned,
		domain.StatusReviewing,
		domain.StatusBlocked,
		domain.StatusPending,
	}

	want := make(map[string]bool, len(domainStatuses))
	for _, status := range domainStatuses {
		want[status.String()] = true
	}

	got := make(map[string]bool, len(auditStatusEnum()))
	for _, name := range auditStatusEnum() {
		if got[name] {
			t.Fatalf("契约枚举里 %q 重复", name)
		}
		got[name] = true
	}

	for name := range want {
		if !got[name] {
			t.Fatalf("domain 有 %q，契约枚举里没有（客户端会落到 default 分支）：%v", name, auditStatusEnum())
		}
	}
	for name := range got {
		if !want[name] {
			t.Fatalf("契约枚举里的 %q 不是任何 domain 状态（客户端会写出走不到的分支）：%v", name, auditStatusEnum())
		}
	}
}

// TestAuditStatusEnumAnnotationIsOnTheWire 直接从 DTO 的 struct tag 读注解，
// 防止"枚举表改了、注解没改"这种只在生成物里才显形的漂移。
func TestAuditStatusEnumAnnotationIsOnTheWire(t *testing.T) {
	t.Parallel()

	want := strings.Join(auditStatusEnum(), ",")
	for _, target := range []any{CreateTaskResponse{}, CompleteTaskResponse{}, TaskStatusResponse{}} {
		field, ok := reflect.TypeOf(target).FieldByName("AuditStatus")
		if !ok {
			t.Fatalf("%T 没有 AuditStatus 字段", target)
		}
		if got := field.Tag.Get("enums"); got != want {
			t.Fatalf("%T 的 enums 注解是 %q，应为 %q", target, got, want)
		}
	}
}
