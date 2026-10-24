package domain

import "strconv"

// 聚合内用到的各类 ID。
//
// 定义独立类型而不是裸 uint64：一旦把 CategoryID 传进需要 TagID 的位置，
// 编译期就会报错。与 models/ctype.ID 的转换放在 adapter 层做。
type (
	// ArticleID 是文章聚合根的标识。
	ArticleID uint64
	// UserID 是文章作者或操作者的标识。
	UserID uint64
	// TagID 是标签标识。
	TagID uint64
	// CategoryID 是分类标识。
	CategoryID uint64
)

// IsZero 判断 ID 是否为空。
func (id ArticleID) IsZero() bool { return id == 0 }

// Uint64 返回底层值，供 adapter 写库使用。
func (id ArticleID) Uint64() uint64 { return uint64(id) }

// String 便于日志与错误信息输出。
func (id ArticleID) String() string { return strconv.FormatUint(uint64(id), 10) }

// IsZero 判断 ID 是否为空。
func (id UserID) IsZero() bool { return id == 0 }

// Uint64 返回底层值。
func (id UserID) Uint64() uint64 { return uint64(id) }

// String 便于日志与错误信息输出。
func (id UserID) String() string { return strconv.FormatUint(uint64(id), 10) }

// Uint64 返回底层值。
func (id TagID) Uint64() uint64 { return uint64(id) }

// Uint64 返回底层值。
func (id CategoryID) Uint64() uint64 { return uint64(id) }
