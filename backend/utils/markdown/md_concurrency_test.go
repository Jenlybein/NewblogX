package markdown

import (
	"fmt"
	"sync"
	"testing"
)

// concurrentSafeInputs 覆盖渲染器会走到的几种节点，确保渲染状态被真正驱动。
var concurrentSafeInputs = []string{
	"正文A",
	"# 标题一\n\n正文B",
	"- 列表一\n- 列表二\n\n正文C",
	"段落一\n\n段落二\n\n[链接](https://example.com)",
	"```go\nfmt.Println(\"hi\")\n```\n\n收尾段落",
	"行内公式 $x^2$ 与块级：\n\n$$\na+b\n$$\n\n结束",
	"![图片](https://example.com/a.png)\n\n图后文字",
	"> 引用块\n>\n> 引用第二段\n\n收尾",
}

// TestMdToSafeConcurrentDeterministic 锁定 MdToSafe 的并发确定性（M4 收尾项 ①）。
//
// 背景：mdrenderer.Renderer 把 previousLineBlank / wasAtNL / source / bufs
// 这类渲染中间状态存在渲染器**实例**上，且 Render 开头并不重置它们。
// 共享单例 engine 时，多个 goroutine 并发调用会互相踩踏，
// 症状是同一输入偶发多出（或少掉）换行。
//
// 修法：把包级单例换成 sync.Pool，让每次调用拿到独立的 goldmark 实例。
// 本测试在修复前必须失败、修复后必须稳定通过。
func TestMdToSafeConcurrentDeterministic(t *testing.T) {
	// 先串行取一次期望值作为基准。
	want := make([]string, len(concurrentSafeInputs))
	for i, in := range concurrentSafeInputs {
		want[i] = MdToSafe(in)
	}

	const goroutines = 32
	const iterations = 100

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		bad  int
		last string
	)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := 0; it < iterations; it++ {
				for i, in := range concurrentSafeInputs {
					got := MdToSafe(in)
					if got == want[i] {
						continue
					}
					mu.Lock()
					bad++
					if last == "" {
						last = fmt.Sprintf("输入 %q:\n want=%q\n got =%q", in, want[i], got)
					}
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	if bad > 0 {
		t.Fatalf("并发下输出不稳定：%d/%d 次与串行基准不一致，示例：\n%s",
			bad, goroutines*iterations*len(concurrentSafeInputs), last)
	}
}

// TestGetRawHTMLConcurrentDeterministic 同上，覆盖走 html 渲染器的那条链路。
func TestGetRawHTMLConcurrentDeterministic(t *testing.T) {
	want := make([]string, len(concurrentSafeInputs))
	for i, in := range concurrentSafeInputs {
		want[i] = getRawHTML(in)
	}

	const goroutines = 32
	const iterations = 100

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		bad  int
		last string
	)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := 0; it < iterations; it++ {
				for i, in := range concurrentSafeInputs {
					got := getRawHTML(in)
					if got == want[i] {
						continue
					}
					mu.Lock()
					bad++
					if last == "" {
						last = fmt.Sprintf("输入 %q:\n want=%q\n got =%q", in, want[i], got)
					}
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	if bad > 0 {
		t.Fatalf("并发下输出不稳定：%d/%d 次与串行基准不一致，示例：\n%s",
			bad, goroutines*iterations*len(concurrentSafeInputs), last)
	}
}
