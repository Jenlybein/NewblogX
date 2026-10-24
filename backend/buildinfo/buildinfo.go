// Package buildinfo 提供构建期注入的版本信息。
//
// 为什么不用常量：写成 const Version = "1.0.0" 之后，
// 线上出的每一个版本都自称 1.0.0，出问题时无法从日志/接口判断跑的是哪个提交。
//
// 正确做法是构建时用 -ldflags 注入，例如：
//
//	go build -ldflags "\
//	  -X myblogx/buildinfo.Version=v1.2.3 \
//	  -X myblogx/buildinfo.Commit=$(git rev-parse --short HEAD) \
//	  -X myblogx/buildinfo.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
//	  -o bin/server ./cmd/server
//
// 未注入时（例如本地 go run）会退化为 "dev"，而不是伪装成一个正式版本号。
package buildinfo

var (
	// Version 语义化版本，来自 git tag。
	Version = "dev"
	// Commit 构建所用的 git 提交短哈希。
	Commit = "unknown"
	// BuildTime 构建时间（UTC，RFC3339）。
	BuildTime = "unknown"
)

// String 返回适合打印在启动日志里的一行摘要。
func String() string {
	return Version + " (" + Commit + ", " + BuildTime + ")"
}
