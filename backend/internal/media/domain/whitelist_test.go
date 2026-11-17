package domain_test

import (
	"reflect"
	"testing"

	"myblogx/internal/media/domain"
)

// TestEffectiveWhitelist 钉住"storage 优先、为空回落 upload"的合并口径。
//
// 这条规则决定上传门禁的放行集合，写歪了的后果是"合法后缀被拒"或"不该放的放进来"，
// 所以每个分支都要有一条用例。
func TestEffectiveWhitelist(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		storage []string
		upload  []string
		want    []string
	}{
		{
			name:    "storage 非空时以 storage 为准",
			storage: []string{"png", "webp"},
			upload:  []string{"jpg", "gif"},
			want:    []string{"png", "webp"},
		},
		{
			name:    "storage 为空回落 upload",
			storage: nil,
			upload:  []string{"jpg", "gif"},
			want:    []string{"jpg", "gif"},
		},
		{
			name:    "storage 只有空白项也算空，回落 upload",
			storage: []string{"", "   "},
			upload:  []string{"png"},
			want:    []string{"png"},
		},
		{
			name:    "两者都空返回空列表",
			storage: nil,
			upload:  nil,
			want:    []string{},
		},
		{
			name:    "归一化：去空白、转小写、去点、去重且保持顺序",
			storage: []string{" PNG ", ".Jpg", "png", "", "gif"},
			upload:  nil,
			want:    []string{"png", "jpg", "gif"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := domain.EffectiveWhitelist(c.storage, c.upload)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("EffectiveWhitelist = %v，期望 %v", got, c.want)
			}
		})
	}
}

// TestEffectiveWhitelistDoesNotMutateInput 保证合并不会就地改写入参。
//
// 入参来自运行时配置快照，就地改会把"后台填的原样"污染成归一化后的样子，
// 下次保存时又会把这份被改过的值写回去。
func TestEffectiveWhitelistDoesNotMutateInput(t *testing.T) {
	t.Parallel()

	storage := []string{" PNG "}
	upload := []string{" jpg "}
	_ = domain.EffectiveWhitelist(storage, upload)

	if storage[0] != " PNG " || upload[0] != " jpg " {
		t.Fatalf("入参被就地改写: storage=%v upload=%v", storage, upload)
	}
}
