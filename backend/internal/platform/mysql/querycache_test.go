package mysql

import "testing"

// 本文件钉住查询缓存键的构造（#132）。
//
// 插件的缓存标识只由「SQL + 参数」组成，不含目标结构体类型。若不补形状，
// 两个 SQL 相同、形状不同的查询会共用同一个键，进而把缺失字段静默归零。

type wideRow struct {
	ID           string
	TokenVersion uint32
}

type narrowRow struct {
	ID string
}

func TestQueryCacheKeySeparatesShapes(t *testing.T) {
	const sql = "SELECT * FROM user_models WHERE id = ?"

	wide := QueryCacheKey(sql, &wideRow{})
	narrow := QueryCacheKey(sql, &narrowRow{})

	if wide == narrow {
		t.Fatalf("不同形状必须产生不同的缓存键，实际都是 %q", wide)
	}
	if wide == sql {
		t.Fatal("形状感知后键不应等于原始键（否则等于没生效）")
	}
}

func TestQueryCacheKeyIsStableForSameShape(t *testing.T) {
	const sql = "SELECT * FROM user_models WHERE id = ?"

	// 同样的形状、不同的值，键必须一致 —— 否则缓存永远命中不了。
	a := QueryCacheKey(sql, &wideRow{ID: "1", TokenVersion: 3})
	b := QueryCacheKey(sql, &wideRow{ID: "2", TokenVersion: 9})

	if a != b {
		t.Fatalf("同形状不同值应产生相同键，实际 %q vs %q", a, b)
	}
}

func TestQueryCacheKeyDistinguishesSliceAndSingle(t *testing.T) {
	const sql = "SELECT * FROM user_models"

	single := QueryCacheKey(sql, &wideRow{})
	slice := QueryCacheKey(sql, &[]wideRow{})

	if single == slice {
		t.Fatal("单值查询与切片查询的 Dest 形状不同，键必须不同")
	}
}

func TestQueryCacheKeyToleratesNilDest(t *testing.T) {
	const sql = "SELECT 1"

	if got := QueryCacheKey(sql, nil); got != sql {
		t.Fatalf("Dest 为空时应原样返回，实际 %q", got)
	}
}
