package app_test

import (
	"context"
	"testing"

	"myblogx/internal/apperr"
	"myblogx/internal/media/app"
	"myblogx/internal/media/apptest"
	"myblogx/internal/media/domain"
)

// 本文件是 #216「上传用途与后端比例校验」的用例接缝：
//   - 创建任务声明用途后，任务里必须记下用途与目标比例；
//   - 七牛回调与前端手动确认必须共用同一条 confirm/verify 链，
//     对同一张真实图片给出一致结论（合规 ready / 不合规 failed + 明确错误码）；
//   - 未声明用途的任务（私信发图）不做比例约束，两条路径都 ready。
//
// 断言的是用例的可观察结果（返回的 Task/Image、错误码、任务状态、图片行是否存在），
// 不是"verify 里调了哪个方法"。

func purposeCfg() domain.Config {
	return domain.Config{
		Enabled: true, Bucket: "bucket", Prefix: "myblogx", Region: "z0",
		SizeMB: 5, ExpirySeconds: 3600,
		CallbackURL: "https://example.com/api/images/qiniu/callback",
		Whitelist:   []string{"png", "jpg", "jpeg"},
	}
}

// ratioStorage 让测试给出对象存储上"真实图片"的宽高；沿用 apptest.Storage
// 的其余行为（Stat 的 hash/size 与任务声明一致），只覆盖 ImageInfo。
type ratioStorage struct {
	apptest.Storage
	width  int
	height int
}

func (s ratioStorage) ImageInfo(_ string) (domain.ImageInfo, error) {
	return domain.ImageInfo{Format: "png", Width: s.width, Height: s.height, Size: 1}, nil
}

// Stat 返回与本文件 createPurposeTask 声明一致的 hash/size，
// 让"手动确认"路径（没有回调元信息，必须回查对象存储）能走通。
func (s ratioStorage) Stat(_, _ string) (domain.ObjectInfo, error) {
	return domain.ObjectInfo{Hash: "hash-1", Size: 1}, nil
}

var _ domain.ObjectStorage = ratioStorage{}

func newPurposeService(width, height int) (*app.Service, *apptest.Repo, *apptest.TaskStore) {
	repo := apptest.NewRepo()
	tasks := apptest.NewTaskStore()
	svc := app.New(repo, tasks, apptest.NewAuditStore(), ratioStorage{width: width, height: height},
		&apptest.IDs{}, apptest.Clock{}, nil, func() domain.Config { return purposeCfg() })
	return svc, repo, tasks
}

// createPurposeTask 的入参是"请求体里的 purpose 字面量"：nil = 没带该字段。
func createPurposeTask(t *testing.T, svc *app.Service, purpose *string) *domain.Task {
	t.Helper()
	res, err := svc.CreateTask(context.Background(), 7, "cover.png", 1, "image/png", "hash-1", purpose)
	if err != nil {
		t.Fatalf("创建上传任务失败: %v", err)
	}
	if res.SkipUpload || res.Task == nil {
		t.Fatalf("应创建一条新任务，实际 %+v", res)
	}
	return res.Task
}

func purposeLiteral(s string) *string { return &s }

func repoImageCount(t *testing.T, repo *apptest.Repo) int64 {
	t.Helper()
	_, total, err := repo.List(context.Background(), domain.Query{})
	if err != nil {
		t.Fatalf("读图片列表失败: %v", err)
	}
	return total
}

// 创建任务声明用途后，任务里必须记下用途与目标比例（封面 16:9 / 头像 1:1）。
func TestCreateTaskRecordsPurposeAndTargetRatio(t *testing.T) {
	t.Parallel()

	cases := []struct {
		purpose string
		want    domain.Ratio
	}{
		{"cover", domain.Ratio{W: 16, H: 9}},
		{"avatar", domain.Ratio{W: 1, H: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.purpose, func(t *testing.T) {
			svc, _, tasks := newPurposeService(1600, 900)
			created := createPurposeTask(t, svc, purposeLiteral(tc.purpose))

			saved, err := tasks.Get(context.Background(), created.ID.Uint64())
			if err != nil {
				t.Fatalf("读回任务失败: %v", err)
			}
			if saved.TargetRatio != tc.want {
				t.Fatalf("任务里的目标比例应为 %+v，实际 %+v", tc.want, saved.TargetRatio)
			}
			if saved.Purpose != domain.Purpose(tc.purpose) {
				t.Fatalf("任务里的用途应为 %q，实际 %q", tc.purpose, saved.Purpose)
			}
		})
	}
}

// 非法用途必须有明确错误码，不能静默当作 cover。
func TestCreateTaskRejectsUnknownPurpose(t *testing.T) {
	t.Parallel()

	svc, _, _ := newPurposeService(1600, 900)
	_, err := svc.CreateTask(context.Background(), 7, "cover.png", 1, "image/png", "hash-1", purposeLiteral("poster"))
	if !apperr.IsCode(err, apperr.CodeImagePurposeInvalid) {
		t.Fatalf("非法用途应返回 image.purpose_invalid，实际 %v", err)
	}
}

// 比例合规 / 不合规 × 回调 / 手动确认：四条路径结论必须一致。
// 期望值来自独立算好的尺寸字面量：1600×900 = 16:9，256×256 = 1:1；
// 900×900 不是 16:9，1600×900 不是 1:1。
func TestConfirmRatioComplianceIsIdenticalForCallbackAndUser(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		purpose   string
		width     int
		height    int
		wantReady bool
		wantErr   apperr.Code
	}{
		{"封面合规通过", "cover", 1600, 900, true, ""},
		{"封面不合规失败", "cover", 900, 900, false, apperr.CodeImageRatioMismatch},
		{"头像合规通过", "avatar", 256, 256, true, ""},
		{"头像不合规失败", "avatar", 1600, 900, false, apperr.CodeImageRatioMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range []string{"callback", "user"} {
				svc, repo, tasks := newPurposeService(tc.width, tc.height)
				created := createPurposeTask(t, svc, purposeLiteral(tc.purpose))

				res, err := confirmPurposeTask(svc, created, path)

				if tc.wantReady {
					if err != nil {
						t.Fatalf("%s: 合规图片应确认成功，实际 %v", path, err)
					}
					if res.Task.Status != domain.TaskReady || res.Image == nil {
						t.Fatalf("%s: 应返回 ready 任务与图片，实际 %+v", path, res)
					}
					if res.Image.Width != tc.width || res.Image.Height != tc.height {
						t.Fatalf("%s: 图片行应记录真实尺寸 %d×%d，实际 %d×%d",
							path, tc.width, tc.height, res.Image.Width, res.Image.Height)
					}
					if !repo.Has(res.Image.ID) {
						t.Fatalf("%s: 合规图片应落图片行", path)
					}
					continue
				}

				if !apperr.IsCode(err, tc.wantErr) {
					t.Fatalf("%s: 不合规图片应返回 %s，实际 %v", path, tc.wantErr, err)
				}
				if res != nil {
					t.Fatalf("%s: 失败时不应返回结果，实际 %+v", path, res)
				}
				saved, getErr := tasks.Get(context.Background(), created.ID.Uint64())
				if getErr != nil {
					t.Fatalf("%s: 读回任务失败: %v", path, getErr)
				}
				if saved.Status != domain.TaskFailed {
					t.Fatalf("%s: 不合规应把任务判 failed，实际 %q", path, saved.Status)
				}
				if saved.ErrorMsg != apperr.Message(tc.wantErr) {
					t.Fatalf("%s: 任务错误信息应为明确文案 %q，实际 %q", path, apperr.Message(tc.wantErr), saved.ErrorMsg)
				}
				if saved.ImageID != nil {
					t.Fatalf("%s: 失败任务不应绑定图片", path)
				}
				if got := repoImageCount(t, repo); got != 0 {
					t.Fatalf("%s: 校验失败不应落图片行，实际 %d 行", path, got)
				}
			}
		})
	}
}

// confirmPurposeTask 从两个入口之一确认同一条任务，让"两条路径结果一致"
// 可以在同一张表驱动用例里逐行对照。
func confirmPurposeTask(svc *app.Service, task *domain.Task, path string) (*app.ConfirmResult, error) {
	ctx := context.Background()
	if path == "callback" {
		return svc.ConfirmByCallback(ctx, task.ObjectKey, task.Bucket, task.Hash, task.DeclaredSize)
	}
	return svc.ConfirmByUser(ctx, task.ID.Uint64(), task.UserID.Uint64(), task.ObjectKey)
}

// 未声明用途 → 无比例约束：私信那种任意比例照片在两条确认路径都 ready。
// 这条用例防止将来有人把比例校验改成"无条件执行"。
func TestConfirmWithoutPurposeSkipsRatioCheck(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"callback", "user"} {
		svc, repo, tasks := newPurposeService(1234, 777) // 任意比例
		created := createPurposeTask(t, svc, nil)        // 请求体不带 purpose
		if created.TargetRatio != (domain.Ratio{}) {
			t.Fatalf("未声明用途的任务不该有目标比例，实际 %+v", created.TargetRatio)
		}

		res, err := confirmPurposeTask(svc, created, path)
		if err != nil {
			t.Fatalf("%s: 未声明用途的任意比例图片应确认成功，实际 %v", path, err)
		}
		if res.Task.Status != domain.TaskReady || res.Image == nil || !repo.Has(res.Image.ID) {
			t.Fatalf("%s: 应落图片行并返回 ready，实际 %+v", path, res)
		}
		saved, getErr := tasks.Get(context.Background(), created.ID.Uint64())
		if getErr != nil {
			t.Fatalf("%s: 读回任务失败: %v", path, getErr)
		}
		if saved.Status != domain.TaskReady {
			t.Fatalf("%s: 任务应为 ready，实际 %q", path, saved.Status)
		}
	}
}

// 失败任务的后续确认应稳定返回"任务校验失败"，而不是重新走一遍校验。
func TestFailedRatioTaskStaysFailed(t *testing.T) {
	t.Parallel()

	svc, _, _ := newPurposeService(900, 900)
	created := createPurposeTask(t, svc, purposeLiteral("cover"))
	if _, err := confirmPurposeTask(svc, created, "callback"); !apperr.IsCode(err, apperr.CodeImageRatioMismatch) {
		t.Fatalf("第一次确认应因比例失败，实际 %v", err)
	}
	if _, err := confirmPurposeTask(svc, created, "user"); !apperr.IsCode(err, apperr.CodeImageTaskFailed) {
		t.Fatalf("二次确认应返回 image.task_failed，实际 %v", err)
	}
}
