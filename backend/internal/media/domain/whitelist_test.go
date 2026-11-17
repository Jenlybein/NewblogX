package domain_test

import (
	"reflect"
	"testing"

	"myblogx/internal/media/domain"
)

// TestEffectiveWhitelist 钉住归一化口径：小写、去空白、去点、去重、保序。
//
// 「上传策略」段退场后（票 418）白名单只有一个来源，所以这里不再有
// "storage 优先 / 回落 upload" 的分支 —— 那条规则连着段一起删掉了。
func TestEffectiveWhitelist(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		storage []string
		want    []string
	}{
		{"大小写与空白", []string{" PNG ", "jpg"}, []string{"png", "jpg"}},
		{"去重保序", []string{"gif", "png", "gif"}, []string{"gif", "png"}},
		{"带点的写法也认", []string{".webp", "bmp"}, []string{"webp", "bmp"}},
		{"空项被丢掉", []string{"", "  ", "png"}, []string{"png"}},
		{"没配就是空", nil, []string{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := domain.EffectiveWhitelist(c.storage)
			if len(got) != len(c.want) {
				t.Fatalf("EffectiveWhitelist = %v，期望 %v", got, c.want)
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Fatalf("第 %d 项 = %q，期望 %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

// TestEffectiveWhitelistDoesNotMutateInput 保证归一化不会就地改写入参。
//
// 入参直接来自运行时配置的缓存对象；就地改写会让"后台看到的"与"实际生效的"
// 悄悄分叉，而且下一次保存会把改过的值写回库。
func TestEffectiveWhitelistDoesNotMutateInput(t *testing.T) {
	t.Parallel()

	storage := []string{" PNG ", "png"}
	before := append([]string(nil), storage...)

	_ = domain.EffectiveWhitelist(storage)

	if !reflect.DeepEqual(storage, before) {
		t.Fatalf("入参被改写了: %v", storage)
	}
}
