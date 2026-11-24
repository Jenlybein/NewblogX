package qiniu

import (
	"errors"
	"testing"

	"github.com/qiniu/go-sdk/v7/storage"
)

// 删除必须是幂等的：目标状态是"对象不存在"，所以"本来就不存在"也算成功。
//
// 不吞 612 会留下一条永远删不掉的记录 —— 图片被判违规时对象已经被删过一次
// （audit_auto_delete_block），运营再点「删除」就每次都失败：2026-11-25 生产上
// 的表现是后台点删除"没反应"，而 DB 里那条 block 行再也清不掉。

func TestIsNoSuchFileRecognisesQiniuCodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "612 文件不存在", err: &storage.ErrorInfo{Code: 612}, want: true},
		{name: "631 桶不存在", err: &storage.ErrorInfo{Code: 631}, want: true},
		{name: "614 目标已存在（不是这件事）", err: &storage.ErrorInfo{Code: 614}, want: false},
		{name: "文案兜底", err: errors.New("no such file or directory"), want: true},
		{name: "其它错误", err: errors.New("bad token"), want: false},
	}

	for _, c := range cases {
		if got := isNoSuchFile(c.err); got != c.want {
			t.Fatalf("%s: isNoSuchFile(%v) = %v，期望 %v", c.name, c.err, got, c.want)
		}
	}
}
