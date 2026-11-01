// 查询缓存的键构造。
//
// 单独成文件是因为它有两个调用方，必须只有一份实现：
//   - core/init_db.go：生产装载查询缓存插件时用它
//   - testkit/mysqlcontainer：契约测试装载同一个插件时用它
//
// 两边若各写一份，测试验证的就不是生产行为，缺陷会重新溜过去。

package mysql

import "fmt"

// QueryCacheKey 把"目标结构体的形状"混进查询缓存键。
//
// # 为什么必须这么做
//
// go-gorm/caches 插件的缓存标识**只由「SQL + 参数」组成，不含目标结构体
// 类型**（见其 identifier.go 的 buildIdentifier：`prefix + query + args`）。
// 命中时它把缓存的 JSON 用 json.Unmarshal 解进**本次查询的 Dest**。
//
// 于是两个 SQL 相同、形状不同的查询会共用同一个键：
// 窄结构体查询（只声明展示用列）往缓存里存一份**少了字段**的 JSON，
// 紧接着的整行查询命中它、解进整行结构体 → 缺失字段**静默归零**（不报错，
// 因为 JSON 里缺字段在解码时是合法的）。
//
// 线上表现为：刷新令牌签发出 token_version=0 的访问令牌（#132 / #99）。
//
// Cacher 拿得到 Dest，所以在这一层把形状补进键里 —— 不必修改插件内部，
// 也不用替换插件。
//
// 用类型名（%T）而不是值：值随行内容变化，那样缓存永远命中不了。
func QueryCacheKey(key string, dest any) string {
	if dest == nil {
		return key
	}
	return fmt.Sprintf("%s|shape=%T", key, dest)
}
