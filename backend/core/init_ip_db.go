// IP 归属地数据库初始化。
//
// 历史背景：这里的 `InitIPDB` 原本**从未被任何地方调用**（2026-10-26 用
// `deadcode` 工具查出来的），而 `utils/requestmeta` 一直在调用 `ipmeta.GetAddr`。
// 后果是 searcher 始终为 nil，`GetAddr` 对任何真实 IP 都返回「未知地址」——
// 登录日志和用户列表里的 `addr` 字段全是这四个字，而仓库里那两个 47MB 的
// xdb 库从来没被加载过。
package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"myblogx/utils/ipmeta"
)

const (
	ipv4DBName = "ip2region_v4.xdb"
	ipv6DBName = "ip2region_v6.xdb"
)

// InitIPDB 加载 IP 归属地库，失败返回错误由调用方决定如何处理。
//
// 为什么返回错误而不是直接 Fatal：`addr` 只是展示字段，库文件缺失时
// `GetAddr` 会降级成「未知地址」，不该因此让整个服务起不来
// （从非 backend 目录启动、或精简镜像没带 resources 时尤其常见）。
func InitIPDB() error {
	dir, err := findIPBaseDir()
	if err != nil {
		return err
	}
	return ipmeta.Init(filepath.Join(dir, ipv4DBName), filepath.Join(dir, ipv6DBName))
}

// findIPBaseDir 定位 resources/ipbase 目录。
//
// 不能写死一个相对路径：服务在容器里以 `/app` 为 CWD 启动
// （Dockerfile 是 `WORKDIR /app` + `COPY resources /app/resources`），
// 而 `go test ./core/` 的 CWD 是 `backend/core`。向上最多探 3 层能同时覆盖
// 这两种情况，以及从仓库根目录执行二进制的场景。
func findIPBaseDir() (string, error) {
	candidates := []string{filepath.Join("resources", "ipbase")}
	for i := 1; i <= 3; i++ {
		candidates = append(candidates,
			filepath.Join(strings.Repeat("../", i), "resources", "ipbase"))
	}
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, ipv4DBName)); err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("未找到 IP 归属地库（已尝试 %v）", candidates)
}

// GetIpAddr 返回 IP 对应的归属地描述。库未加载时返回「未知地址」。
func GetIpAddr(ip string) (addr string) {
	return ipmeta.GetAddr(ip)
}
