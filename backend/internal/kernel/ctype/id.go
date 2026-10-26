package ctype

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// ID 统一使用字符串语义，避免雪花 ID 经过前端 Number 后丢失精度。
//
// **底层就是 string**（曾经是 uint64）。这一层改动的意义：
//
//	以前：类型是 uint64，但 MarshalJSON 会把它输出成字符串。
//	      "Go 类型"与"JSON 里的样子"是两个东西，靠一个 MarshalJSON 偷偷转换。
//	      swag 读的是 Go 类型，于是契约里印成 integer，与实际传输不符，
//	      只能靠生成后的脚本再把 integer 改成 string。
//
//	现在：类型与 JSON 是同一个东西。swag 读类型声明得到 string，不可能印错。
//
// 数据库侧不受影响：Value/Scan 仍然按 int64 读写，DDL 一行未改（零 DDL）。
//
// 代价（使用时必须知道）：
//  1. **不要拿 ID 直接比大小**。string 比较下 "10" < "9" 成立，数值排序会乱。
//     需要比较/排序时用 Uint64()。
//  2. **不要用 ID(123) 构造**。integer→string 转换在 Go 里是合法的
//     （得到的是该码点对应的字符），编译器不会报错但值是错的。
//     一律用 NewID(123)。
type ID string

// NewID 由数值构造 ID。0 会得到零值（空串），保证 IsZero 与 JSON 往返一致。
func NewID(v uint64) ID {
	if v == 0 {
		return ID("")
	}
	return ID(strconv.FormatUint(v, 10))
}

// ParseID 解析字符串形式的 ID；空串得到零值。
func ParseID(raw string) (ID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ID(""), nil
	}
	num, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return ID(""), fmt.Errorf("id 不是合法的数字: %q", raw)
	}
	return NewID(num), nil
}

// Uint64 将雪花 ID 转回数值。
//
// 解析失败（脏数据）时返回 0 而不是 panic：ID 可能来自 URL 参数或外部 JSON，
// 让一处坏数据拖垮整个列表渲染不划算。
func (id ID) Uint64() uint64 {
	num, err := strconv.ParseUint(string(id), 10, 64)
	if err != nil {
		return 0
	}
	return num
}

// IsZero 判断雪花 ID 是否为零。
func (id ID) IsZero() bool {
	return id == ""
}

// String 实现 fmt.Stringer 接口。零值返回空串。
func (id ID) String() string {
	return string(id)
}

// MarshalJSON 实现 json.Marshaler 接口
func (id ID) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(id))
}

// UnmarshalJSON 实现 json.Unmarshaler 接口
//
// 同时接受字符串与数字两种形态：字符串是现在的正式形态，数字要兼容
// 存量数据、数据库导出与测试夹具（那里的 ID 还是数字）。
func (id *ID) UnmarshalJSON(data []byte) error {
	if id == nil {
		return fmt.Errorf("id 不能为空")
	}
	if string(data) == "null" {
		*id = ID("")
		return nil
	}

	var str string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		return id.parse(str)
	}

	var num uint64
	if err := json.Unmarshal(data, &num); err != nil {
		return err
	}
	*id = NewID(num)
	return nil
}

// UnmarshalText 实现 encoding.TextUnmarshaler 接口
func (id *ID) UnmarshalText(text []byte) error {
	if id == nil {
		return fmt.Errorf("id 不能为空")
	}
	return id.parse(string(text))
}

// MarshalText 实现 encoding.TextMarshaler 接口
func (id ID) MarshalText() ([]byte, error) {
	return []byte(string(id)), nil
}

// parse 解析字符串
func (id *ID) parse(raw string) error {
	parsed, err := ParseID(raw)
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// Value 实现 driver.Valuer 接口（落库仍是 int64）
func (id ID) Value() (driver.Value, error) {
	return int64(id.Uint64()), nil
}

// Scan 实现 sql.Scanner 接口
func (id *ID) Scan(value any) error {
	if id == nil {
		return fmt.Errorf("id 不能为空")
	}
	switch v := value.(type) {
	case nil:
		*id = ID("")
		return nil
	case int64:
		*id = NewID(uint64(v))
		return nil
	case int32:
		*id = NewID(uint64(v))
		return nil
	case int:
		*id = NewID(uint64(v))
		return nil
	case uint64:
		*id = NewID(v)
		return nil
	case uint32:
		*id = NewID(uint64(v))
		return nil
	case uint:
		*id = NewID(uint64(v))
		return nil
	case []byte:
		return id.parse(string(v))
	case string:
		return id.parse(v)
	default:
		return fmt.Errorf("不支持的 id 类型: %T", value)
	}
}

// GormDataType 实现 gorm.DataTyper 接口
func (ID) GormDataType() string {
	return "snowflake_id"
}

// GormDBDataType 实现 gorm.Dialector 接口
func (ID) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	switch db.Dialector.Name() {
	case "mysql":
		return "bigint unsigned"
	case "sqlite":
		return "integer"
	default:
		return "bigint"
	}
}
