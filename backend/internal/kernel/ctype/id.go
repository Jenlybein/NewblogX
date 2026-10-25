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

// ID 统一使用字符串 JSON 语义，避免雪花 ID 经过前端 Number 后丢失精度。
type ID uint64

// Uint64 将雪花 ID 转换为 uint64
func (id ID) Uint64() uint64 {
	return uint64(id)
}

// IsZero 判断雪花 ID 是否为零
func (id ID) IsZero() bool {
	return id == 0
}

// String 实现 fmt.Stringer 接口
func (id ID) String() string {
	return strconv.FormatUint(uint64(id), 10)
}

// MarshalJSON 实现 json.Marshaler 接口
func (id ID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

// UnmarshalJSON 实现 json.Unmarshaler 接口
func (id *ID) UnmarshalJSON(data []byte) error {
	if id == nil {
		return fmt.Errorf("id 不能为空")
	}
	if string(data) == "null" {
		*id = 0
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
	*id = ID(num)
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
	return []byte(id.String()), nil
}

// parse 解析字符串
func (id *ID) parse(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		*id = 0
		return nil
	}
	num, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return err
	}
	*id = ID(num)
	return nil
}

// Value 实现 driver.Valuer 接口
func (id ID) Value() (driver.Value, error) {
	return int64(id), nil
}

// Scan 实现 sql.Scanner 接口
func (id *ID) Scan(value any) error {
	if id == nil {
		return fmt.Errorf("id 不能为空")
	}
	switch v := value.(type) {
	case nil:
		*id = 0
		return nil
	case int64:
		*id = ID(v)
		return nil
	case int32:
		*id = ID(v)
		return nil
	case int:
		*id = ID(v)
		return nil
	case uint64:
		*id = ID(v)
		return nil
	case uint32:
		*id = ID(v)
		return nil
	case uint:
		*id = ID(v)
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
