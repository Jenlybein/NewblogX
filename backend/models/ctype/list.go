package ctype

import (
	"database/sql/driver"
	"strings"

	"github.com/pingcap/errors"
)

type List []string

// Scan 实现 sql.Scanner 接口，将 Value 扫描到 List 中
func (l *List) Scan(value interface{}) error {
	if value == nil {
		*l = []string{}
		return nil
	}

	switch v := value.(type) {
	case []uint8:
		*l = strings.Split(string(v), ",")
		return nil
	case string:
		*l = strings.Split(v, ",")
		return nil
	default:
		return errors.Errorf("value is not []uint8 or string")
	}
}

// Value 实现 driver.Valuer 接口，将 List 转换字符串，符合 ES 读取格式（mysql没有自带的数组类型）
func (l List) Value() (driver.Value, error) {
	return strings.Join(l, ","), nil
}
