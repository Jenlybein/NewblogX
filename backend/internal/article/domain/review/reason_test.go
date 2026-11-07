package review_test

import (
	"strings"
	"testing"

	"myblogx/internal/kernel/ctype"

	"myblogx/internal/article/domain/review"
)

// 驳回原因 = 固定枚举（值）+ 可选的一句自由文本。
//
// 这一层只回答两件事：**怎么把两者装进已有的 reason 列**（零 DDL：不加列），
// 以及**作者看到的到底是哪句话**。用例层负责"必须选一条"，不在这里。

// TestReasonTextRendersLabelAndNote 作者看到的是"枚举文案 + 自由文本"，
// 而不是一个机器值，也不是审核员敲的原文。
//
// 期望值来自产品口径（枚举文案见 kernel 词表），不是拿同一段代码重算一遍。
func TestReasonTextRendersLabelAndNote(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "枚举加补充说明", raw: "incomplete\n请补一段示例代码", want: "内容不完整：请补一段示例代码"},
		{name: "只选了枚举", raw: "quality", want: "内容质量不达标"},
		{name: "补充说明有多行", raw: "format\n第一行\n第二行", want: "排版格式问题：第一行\n第二行"},
		{name: "枚举后的补充说明首尾空白要去掉", raw: "quality\n\n  排版太散\n", want: "内容质量不达标：排版太散"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := review.ReasonText(c.raw); got != c.want {
				t.Fatalf("作者看到的应为 %q，实际 %q", c.want, got)
			}
		})
	}
}

// TestReasonTextKeepsLegacyFreeText 历史行与撤销原因都是纯自由文本：
// 第一行不是固定枚举值时必须原样呈现，不能把作者看不懂的机器值塞给他，
// 也不能丢掉他原本读到的整段话。
func TestReasonTextKeepsLegacyFreeText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "#303 之前手写的单句", raw: "排版太乱", want: "排版太乱"},
		{name: "历史多句", raw: "上一轮：排版乱\n下一轮：论证缺证据", want: "上一轮：排版乱\n下一轮：论证缺证据"},
		{name: "看似枚举但不是", raw: "quality_v2\n还没上线的新值", want: "quality_v2\n还没上线的新值"},
		{name: "只有空白", raw: " \n\t ", want: review.ReasonMissingFallback},
		{name: "空串", raw: "", want: review.ReasonMissingFallback},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := review.ReasonText(c.raw); got != c.want {
				t.Fatalf("历史原因应渲染为 %q，实际 %q", c.want, got)
			}
		})
	}
}

// TestFormatReasonRoundTrip 编码与解析是同一对逆运算：
// 用例层写进去的东西，读面（管理后台条目 / 作者通知）必须能拆回来。
func TestFormatReasonRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		code       review.ReasonCode
		note       string
		wantStored string
	}{
		{name: "枚举 + 补充", code: review.ReasonIncomplete, note: "缺少示例代码", wantStored: "incomplete\n缺少示例代码"},
		{name: "只有枚举", code: review.ReasonQuality, note: "", wantStored: "quality"},
		{name: "补充说明的首尾空白落库时去掉", code: review.ReasonFormat, note: "  排版太散  ", wantStored: "format\n排版太散"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			stored := review.FormatReason(c.code, c.note)
			if stored != c.wantStored {
				t.Fatalf("落库内容应为 %q，实际 %q", c.wantStored, stored)
			}
			code, note := review.ParseReason(stored)
			if code != c.code {
				t.Fatalf("应解析回枚举 %q，实际 %q", c.code, code)
			}
			if note != strings.TrimSpace(c.note) {
				t.Fatalf("应解析回补充说明 %q，实际 %q", strings.TrimSpace(c.note), note)
			}
		})
	}
}

// TestParseReasonLegacyRow 历史行的第一行是自由文本，不是枚举值：
// 解析结果必须是"没有枚举、整串都是文本"，这样管理后台才知道该按哪条路渲染。
func TestParseReasonLegacyRow(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"", "排版太乱", "上一轮：排版乱\n下一轮：论证缺证据", "quality_v2"} {
		code, note := review.ParseReason(raw)
		if code != "" {
			t.Fatalf("%q 不该被认成枚举，实际 %q", raw, code)
		}
		if note != strings.TrimSpace(raw) {
			t.Fatalf("%q 应整串当文本，实际 %q", raw, note)
		}
	}
}

// TestReasonCodesCarryLabel 词表要能覆盖到文案：
// 缺一行文案时下拉框会出现一个空选项，作者收到的也会是一句空话。
func TestReasonCodesCarryLabel(t *testing.T) {
	t.Parallel()

	codes := review.ReasonCodes()
	if len(codes) == 0 {
		t.Fatal("固定枚举不能为空")
	}
	seen := make(map[review.ReasonCode]bool, len(codes))
	for _, code := range codes {
		if code == "" {
			t.Fatal("枚举值不能为空串（空串表示「没有枚举」，历史行在用）")
		}
		if seen[code] {
			t.Fatalf("枚举值重复: %q", code)
		}
		seen[code] = true
		if label := review.ReasonLabel(code); label == "" {
			t.Fatalf("枚举 %q 缺中文文案", code)
		}
	}
	if label := review.ReasonLabel("quality"); label != "内容质量不达标" {
		t.Fatalf("quality 的文案应稳定为「内容质量不达标」，实际 %q", label)
	}
}

// TestRejectStoresEnumAndNote 任务是唯一写 reason 列的地方：
// 驳回必须把"枚举 + 补充"编码进去，读面才不用猜。
func TestRejectStoresEnumAndNote(t *testing.T) {
	t.Parallel()

	task := newTask()
	if err := task.Reject(ctype.NewID(900), review.ReasonCompliance, "涉及未经证实的指控", now); err != nil {
		t.Fatalf("驳回失败: %v", err)
	}
	if got := task.ReasonCode(); got != review.ReasonCompliance {
		t.Fatalf("任务上的枚举应为 %q，实际 %q", review.ReasonCompliance, got)
	}
	if got := task.ReasonNote(); got != "涉及未经证实的指控" {
		t.Fatalf("任务上的补充说明应为 %q，实际 %q", "涉及未经证实的指控", got)
	}
	if got := review.ReasonText(task.Reason()); got != "内容不合规：涉及未经证实的指控" {
		t.Fatalf("作者看到的那句话不对: %q", got)
	}
}
