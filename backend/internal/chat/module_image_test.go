package chat

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"myblogx/conf"
	"myblogx/internal/kernel/ctype"
	sitecfg "myblogx/internal/platform/config"
	"myblogx/test/testutil"
)

// chatImageVerifier 的判定（票 267）：只认「能按当前存储配置反解出 object_key
// 且在 image_models 里有记录」的 URL。外链 / 拼错前缀 / 未确认的 key 一律 false。
//
// 这里不造新表：用 chat 模块自己那份 image_models 行结构在 sqlite 里建表即可，
// 与生产查的是同一张表、同一列。

type verifierImageRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	ObjectKey string
	DeletedAt gorm.DeletedAt
}

func (verifierImageRow) TableName() string { return "image_models" }

func newVerifier(t *testing.T, storage conf.QiNiu) *imageVerifier {
	t.Helper()
	db := testutil.SetupSQLite(t, &verifierImageRow{})
	site := sitecfg.NewRuntimeConfigService(conf.RuntimeDefaults{Storage: storage}, nil, nil)
	return &imageVerifier{db: db, storage: site}
}

func TestChatImageVerifierOnlyAcceptsOwnedUploads(t *testing.T) {
	cfg := conf.QiNiu{Enable: true, Prefix: "myblogx", Uri: "https://cdn.example.com"}
	v := newVerifier(t, cfg)
	if err := v.db.Create(&verifierImageRow{ID: ctype.NewID(1), ObjectKey: "myblogx/images/20261104/abc.png"}).Error; err != nil {
		t.Fatalf("建图片行失败: %v", err)
	}
	if err := v.db.Create(&verifierImageRow{ID: ctype.NewID(2), ObjectKey: "myblogx/images/20261104/gone.png",
		DeletedAt: gorm.DeletedAt{Valid: true}}).Error; err != nil {
		t.Fatalf("建软删图片行失败: %v", err)
	}

	cases := []struct {
		name string
		url  string
		want bool
	}{
		{"本站已确认的图片", "https://cdn.example.com/myblogx/images/20261104/abc.png", true},
		{"外链域名", "https://evil.example.com/myblogx/images/20261104/abc.png", false},
		{"本站域名但没有对应记录", "https://cdn.example.com/myblogx/images/20261104/unknown.png", false},
		{"前缀不对", "https://cdn.example.com/other/images/20261104/abc.png", false},
		{"已软删的图片", "https://cdn.example.com/myblogx/images/20261104/gone.png", false},
		{"裸外链像素", "https://evil.example.com/pixel.png", false},
		{"空串", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := v.IsOwnedImageURL(context.Background(), c.url)
			if err != nil {
				t.Fatalf("判定报错: %v", err)
			}
			if got != c.want {
				t.Fatalf("url=%q 期望 %v，实际 %v", c.url, c.want, got)
			}
		})
	}
}

func TestChatImageVerifierFailsClosedWhenStorageDisabled(t *testing.T) {
	v := newVerifier(t, conf.QiNiu{Enable: false, Prefix: "myblogx"})
	if err := v.db.Create(&verifierImageRow{ID: ctype.NewID(1), ObjectKey: "myblogx/images/20261104/abc.png"}).Error; err != nil {
		t.Fatalf("建图片行失败: %v", err)
	}
	got, err := v.IsOwnedImageURL(context.Background(), "https://cdn.example.com/myblogx/images/20261104/abc.png")
	if err != nil {
		t.Fatalf("判定报错: %v", err)
	}
	if got {
		t.Fatal("对象存储未启用时应失败关闭（不认任何图片）")
	}
}
