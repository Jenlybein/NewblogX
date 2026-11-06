//go:build integration

package mysql_test

import (
	"context"
	"net/http"
	"testing"

	"gorm.io/gorm"

	"myblogx/internal/apperr"
	mediamysql "myblogx/internal/media/adapter/mysql"
	"myblogx/internal/media/app"
	mediaapptest "myblogx/internal/media/apptest"
	"myblogx/internal/media/domain"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// #216 真 MySQL 集成用例：确认时按用途校验真实图片比例，合规落图片行
// （带真实宽高），不合规把任务判 failed 且**不落图片行**。
//
// 走真建表（image_models / image_ref_models）+ 真 Repo，任务缓存与对象存储
// 用有状态假件：比例校验的输入来自 storage.ImageInfo（对象存储上的真实尺寸），
// 这正是生产 confirm/verify 链取尺寸的地方。
//
// 本机没有 Docker，此文件只在 `go vet -tags integration` 下编译验证，
// 不实机执行；CI 有 Docker 时由 `go test -tags integration ./...` 跑。

type integrationStorage struct {
	width  int
	height int
}

var _ domain.ObjectStorage = integrationStorage{}

func (s integrationStorage) Token(p domain.Policy) (domain.Token, error) {
	return domain.Token{Token: "token", Bucket: p.Bucket, ObjectKey: p.ObjectKey, ExpireAt: p.ExpireAt}, nil
}
func (integrationStorage) VerifyCallback(_ *http.Request) (bool, error) { return true, nil }
func (integrationStorage) Stat(_, _ string) (domain.ObjectInfo, error) {
	return domain.ObjectInfo{Hash: "it-hash", Size: 1}, nil
}
func (integrationStorage) Delete(_, _ string) error { return nil }
func (s integrationStorage) ImageInfo(_ string) (domain.ImageInfo, error) {
	return domain.ImageInfo{Format: "png", Width: s.width, Height: s.height, Size: 1}, nil
}
func (integrationStorage) ObjectURL(key string) string {
	return "https://image.example.com/" + key
}

func integrationCfg() domain.Config {
	return domain.Config{
		Enabled: true, Bucket: "bucket", Prefix: "myblogx", Region: "z0",
		SizeMB: 5, ExpirySeconds: 3600,
		CallbackURL: "https://example.com/api/images/qiniu/callback",
		Whitelist:   []string{"png"},
	}
}

func setupPurposeIntegration(t *testing.T, width, height int) (*app.Service, *gorm.DB, *mediaapptest.TaskStore) {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.ImageAssets)
	repo := mediamysql.New(db)
	tasks := mediaapptest.NewTaskStore()
	svc := app.New(repo, tasks, mediaapptest.NewAuditStore(), integrationStorage{width: width, height: height},
		&mediaapptest.IDs{}, mediaapptest.Clock{}, nil, func() domain.Config { return integrationCfg() })
	return svc, db, tasks
}

func countImageRows(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Table("image_models").Where("deleted_at IS NULL").Count(&n).Error; err != nil {
		t.Fatalf("数图片行失败: %v", err)
	}
	return n
}

// createIntegrationTask 的 purpose 是"请求体里的字面量"：空串表示请求体没带该字段。
func createIntegrationTask(t *testing.T, svc *app.Service, purpose string) *domain.Task {
	t.Helper()
	var declared *string
	if purpose != "" {
		declared = &purpose
	}
	res, err := svc.CreateTask(context.Background(), 7, "picture.png", 1, "image/png", "it-hash", declared)
	if err != nil {
		t.Fatalf("创建上传任务失败: %v", err)
	}
	return res.Task
}

// 合规通过：封面 1600×900、头像 256×256 都落图片行并写入真实尺寸。
func TestPurposeRatioCompliantUploadPersistsImage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		purpose string
		width   int
		height  int
	}{
		{"封面 16:9", "cover", 1600, 900},
		{"头像 1:1", "avatar", 256, 256},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, db, tasks := setupPurposeIntegration(t, tc.width, tc.height)
			created := createIntegrationTask(t, svc, tc.purpose)

			res, err := svc.ConfirmByCallback(context.Background(), created.ObjectKey, created.Bucket, created.Hash, created.DeclaredSize)
			if err != nil {
				t.Fatalf("合规图片应确认成功: %v", err)
			}
			if res.Task.Status != domain.TaskReady {
				t.Fatalf("任务应为 ready，实际 %q", res.Task.Status)
			}
			if countImageRows(t, db) != 1 {
				t.Fatalf("合规确认后应恰好落 1 行图片，实际 %d", countImageRows(t, db))
			}

			var row schema.ImageRow
			if err := db.Where("hash = ?", "it-hash").Take(&row).Error; err != nil {
				t.Fatalf("读回图片行失败: %v", err)
			}
			if row.Width != tc.width || row.Height != tc.height {
				t.Fatalf("图片行应记录真实尺寸 %d×%d，实际 %d×%d", tc.width, tc.height, row.Width, row.Height)
			}
			saved, err := tasks.Get(context.Background(), created.ID.Uint64())
			if err != nil || saved.Status != domain.TaskReady {
				t.Fatalf("任务缓存应为 ready: task=%+v err=%v", saved, err)
			}
		})
	}
}

// 不合规失败：任务 failed + 明确错误码，且图片行一行都不能有。
// 回调与手动确认两条路径都必须得到同一结论。
func TestPurposeRatioMismatchFailsTaskAndPersistsNothing(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		purpose string
		width   int
		height  int
	}{
		{"封面拿方图", "cover", 900, 900},
		{"头像拿宽图", "avatar", 1600, 900},
	}
	for _, tc := range cases {
		for _, path := range []string{"callback", "user"} {
			t.Run(tc.name+"/"+path, func(t *testing.T) {
				svc, db, tasks := setupPurposeIntegration(t, tc.width, tc.height)
				created := createIntegrationTask(t, svc, tc.purpose)

				var err error
				if path == "callback" {
					_, err = svc.ConfirmByCallback(context.Background(), created.ObjectKey, created.Bucket, created.Hash, created.DeclaredSize)
				} else {
					_, err = svc.ConfirmByUser(context.Background(), created.ID.Uint64(), created.UserID.Uint64(), created.ObjectKey)
				}
				if !apperr.IsCode(err, apperr.CodeImageRatioMismatch) {
					t.Fatalf("不合规应返回 image.ratio_mismatch，实际 %v", err)
				}
				saved, getErr := tasks.Get(context.Background(), created.ID.Uint64())
				if getErr != nil {
					t.Fatalf("读回任务失败: %v", getErr)
				}
				if saved.Status != domain.TaskFailed || saved.ErrorMsg != apperr.Message(apperr.CodeImageRatioMismatch) {
					t.Fatalf("任务应 failed 且带明确文案，实际 %+v", saved)
				}
				if got := countImageRows(t, db); got != 0 {
					t.Fatalf("校验失败不应落图片行，实际 %d 行", got)
				}
			})
		}
	}
}

// 未声明用途（私信发图）任意比例也通过，且落图片行。
func TestNoPurposeSkipsRatioCheckOnRealMySQL(t *testing.T) {
	t.Parallel()

	svc, db, _ := setupPurposeIntegration(t, 1234, 777)
	created := createIntegrationTask(t, svc, "")

	if _, err := svc.ConfirmByUser(context.Background(), created.ID.Uint64(), created.UserID.Uint64(), created.ObjectKey); err != nil {
		t.Fatalf("未声明用途的任意比例图片应确认成功: %v", err)
	}
	if got := countImageRows(t, db); got != 1 {
		t.Fatalf("未声明用途也应正常落图片行，实际 %d 行", got)
	}
}
