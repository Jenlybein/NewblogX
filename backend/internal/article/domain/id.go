package domain

import (
	"myblogx/internal/kernel/ctype"
)

// 聚合内用到的各类 ID。
//
// 全部是 ctype.ID 的别名，不是独立类型。
//
// 为什么改成别名：以前这里是 4 个独立的 `uint64` 类型，与 ctype.ID 之间
// 要靠显式强转互相倒腾（全后端 192 处）。改成别名后它们是同一个类型，
// 那 192 处转换直接消失；同时 ID 底层变成 string，契约里 swag 读到的
// 就是 string，不需要生成后再修补。
//
// 代价：ArticleID 与 UserID 之间不再有编译期区分。这两个类型本来就靠
// uint64 强转互通，保护形同虚设，不值得为它保留 192 处噪音。
//
// 方法（IsZero / Uint64 / String）都由 ctype.ID 提供，不再重复定义。
type (
	// ArticleID 是文章聚合根的标识。
	ArticleID = ctype.ID
	// UserID 是文章作者或操作者的标识。
	UserID = ctype.ID
	// TagID 是标签标识。
	TagID = ctype.ID
	// CategoryID 是分类标识。
	CategoryID = ctype.ID
)
