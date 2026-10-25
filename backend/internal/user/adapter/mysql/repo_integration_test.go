//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/user/adapter/mysql"
	"myblogx/internal/user/domain"
	"myblogx/models"

	"gorm.io/gorm"
)

// 这些是契约测试：打真 MySQL，验证 row 结构与真实表结构一致。
//
// 为什么不能只跑内存 fake：fake 验证的是"逻辑对不对"，验证不了
// "列名写错没有""TableName 对不对""ON CONFLICT 语法在 MySQL 上生不生效"。
// 这三类错误在 fake 下永远绿，上线才炸。
//
// 建表直接复用 models 里的定义：如果本 adapter 的 row 与 models 的列名漂移，
// 这里的读写会立刻失败——这正是我们想要的保护。

// env 把一次测试需要的适配器聚在一起（不叫 Options：archcheck R2 会报）。
type env struct {
	repo     *mysql.Repo
	settings *mysql.SettingsRepo
	stats    *mysql.StatsRepo
	db       *gorm.DB
	ctx      context.Context
}

func setup(t *testing.T) *env {
	t.Helper()
	db := mysqlcontainer.DB(t,
		&models.UserModel{},
		&models.UserConfModel{},
		&models.UserStatModel{},
	)
	// 快照表用原生 DDL 建最小结构：AutoMigrate 会顺带建出指向 article/user 的外键，
	// 而本模块只写其中 2-3 个冗余列，不该为了测试去造一整套关联数据。
	for _, ddl := range []string{
		`CREATE TABLE comment_models (id bigint PRIMARY KEY, user_id bigint, user_nickname varchar(64),
			user_avatar varchar(256), reply_user_id bigint, reply_user_nickname varchar(64))`,
		`CREATE TABLE user_follow_models (id bigint PRIMARY KEY, followed_user_id bigint, followed_nickname varchar(64),
			followed_avatar varchar(256), followed_abstract varchar(256), fans_user_id bigint, fans_nickname varchar(64),
			fans_avatar varchar(256), fans_abstract varchar(256))`,
		`CREATE TABLE favorite_models (id bigint PRIMARY KEY, user_id bigint, owner_nickname varchar(64),
			owner_avatar varchar(256))`,
		`CREATE TABLE user_article_favor_models (id bigint PRIMARY KEY, article_author_id bigint,
			article_author_nickname varchar(64), article_author_avatar varchar(256))`,
		`CREATE TABLE chat_session_models (id bigint PRIMARY KEY, receiver_id bigint, receiver_nickname varchar(64),
			receiver_avatar varchar(256))`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("建快照表失败: %v", err)
		}
	}
	return &env{
		repo:     mysql.New(db),
		settings: mysql.NewSettings(db),
		stats:    mysql.NewStats(db),
		db:       db,
		ctx:      context.Background(),
	}
}

func newUser(name string) domain.User {
	email := name + "@example.com"
	return domain.User{
		Username:     name,
		Nickname:     "昵称-" + name,
		Avatar:       "https://cdn/" + name + ".png",
		Abstract:     "简介",
		Email:        &email,
		PasswordHash: "hash",
		Status:       domain.StatusActive,
		Role:         domain.RoleUser,
		TokenVersion: 1,
		CreatedAt:    time.Now(),
	}
}

func TestRepoCreateAndFind(t *testing.T) {
	t.Parallel()
	e := setup(t)
	repo, ctx := e.repo, e.ctx

	u := newUser("alice")
	if err := repo.Create(ctx, &u); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if u.ID.IsZero() {
		t.Fatal("雪花 ID 未回填")
	}

	got, err := repo.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("按 ID 查询失败: %v", err)
	}
	if got.Username != "alice" || got.Nickname != "昵称-alice" || got.Email == nil {
		t.Fatalf("往返字段不一致: %+v", got)
	}
	if got.Status != domain.StatusActive {
		t.Fatalf("默认状态应为正常，实际: %v", got.Status)
	}

	if _, err := repo.FindByUsername(ctx, "alice"); err != nil {
		t.Fatalf("按用户名查询失败: %v", err)
	}
	if _, err := repo.FindByEmail(ctx, "alice@example.com"); err != nil {
		t.Fatalf("按邮箱查询失败: %v", err)
	}
}

func TestRepoFindMissing(t *testing.T) {
	t.Parallel()
	e := setup(t)
	repo, ctx := e.repo, e.ctx
	_, err := repo.FindByID(ctx, 999999)
	if code, ok := apperr.CodeOf(err); !ok || code != apperr.CodeUserNotFound {
		t.Fatalf("不存在时应返回 CodeUserNotFound，实际: %v", err)
	}
}

func TestRepoExists(t *testing.T) {
	t.Parallel()
	e := setup(t)
	repo, ctx := e.repo, e.ctx
	u := newUser("bob")
	if err := repo.Create(ctx, &u); err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	if ok, err := repo.ExistsUsername(ctx, "bob", 0); err != nil || !ok {
		t.Fatalf("应判定为已占用: ok=%v err=%v", ok, err)
	}
	if ok, _ := repo.ExistsUsername(ctx, "bob", u.ID); ok {
		t.Fatal("排除自己时应判定为未占用")
	}
	if ok, _ := repo.ExistsUsername(ctx, "nobody", 0); ok {
		t.Fatal("不存在的用户名不该占用")
	}
	if ok, _ := repo.ExistsEmail(ctx, "bob@example.com", 0); !ok {
		t.Fatal("邮箱应判定为已占用")
	}
}

func TestRepoUpdateProfile(t *testing.T) {
	t.Parallel()
	e := setup(t)
	repo, settings, ctx := e.repo, e.settings, e.ctx
	u := newUser("carol")
	if err := repo.Create(ctx, &u); err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	nick := "新昵称"
	abs := ""
	at := time.Now()
	if err := repo.UpdateProfile(ctx, u.ID, domain.ProfileUpdate{Nickname: &nick, Abstract: &abs}, &at); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	got, _ := repo.FindByID(ctx, u.ID)
	if got.Nickname != "新昵称" {
		t.Fatalf("昵称未更新: %q", got.Nickname)
	}
	if got.Abstract != "" {
		t.Fatalf("简介应被清空: %q", got.Abstract)
	}
	if got.Username != "carol" {
		t.Fatalf("未请求修改的用户名不该变: %q", got.Username)
	}

	// 冷却时间戳应落在 user_conf 表。
	s, err := settings.Find(ctx, u.ID)
	if err != nil {
		t.Fatalf("读偏好失败: %v", err)
	}
	if s.UpdatedUsernameAt == nil {
		t.Fatal("冷却时间戳未写入")
	}
}

func TestRepoUpdateStatus(t *testing.T) {
	t.Parallel()
	e := setup(t)
	repo, ctx := e.repo, e.ctx
	u := newUser("dave")
	if err := repo.Create(ctx, &u); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if err := repo.UpdateStatus(ctx, u.ID, domain.StatusBanned); err != nil {
		t.Fatalf("改状态失败: %v", err)
	}
	got, _ := repo.FindByID(ctx, u.ID)
	if got.Status != domain.StatusBanned {
		t.Fatalf("状态未更新: %v", got.Status)
	}
	if err := repo.UpdateStatus(ctx, 999999, domain.StatusActive); err == nil {
		t.Fatal("不存在的用户应报错")
	}
}

func TestRepoList(t *testing.T) {
	t.Parallel()
	e := setup(t)
	repo, ctx := e.repo, e.ctx
	for _, name := range []string{"u1", "u2", "u3"} {
		u := newUser(name)
		if err := repo.Create(ctx, &u); err != nil {
			t.Fatalf("创建失败: %v", err)
		}
	}

	page, err := repo.List(ctx, domain.Query{Page: 1, Size: 2})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if page.Total != 3 || len(page.Items) != 2 {
		t.Fatalf("分页不对: total=%d len=%d", page.Total, len(page.Items))
	}

	page, err = repo.List(ctx, domain.Query{Keyword: "u2"})
	if err != nil {
		t.Fatalf("关键字查询失败: %v", err)
	}
	if page.Total != 1 || page.Items[0].Username != "u2" {
		t.Fatalf("关键字过滤不对: %+v", page)
	}

	disabled := domain.StatusDisabled
	page, _ = repo.List(ctx, domain.Query{Status: &disabled})
	if page.Total != 0 {
		t.Fatalf("状态过滤不对: %d", page.Total)
	}
}

func TestSettingsAndStats(t *testing.T) {
	t.Parallel()
	e := setup(t)
	repo, settings, stats, ctx := e.repo, e.settings, e.stats, e.ctx
	u := newUser("erin")
	if err := repo.Create(ctx, &u); err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	t.Run("未建档时返回默认值", func(t *testing.T) {
		s, err := settings.Find(ctx, u.ID)
		if err != nil {
			t.Fatalf("读默认偏好失败: %v", err)
		}
		if !s.FavoritesVisibility || s.HomeStyleID != 1 {
			t.Fatalf("默认值不对: %+v", s)
		}
	})

	t.Run("InitDefaults 幂等", func(t *testing.T) {
		if err := settings.InitDefaults(ctx, u.ID); err != nil {
			t.Fatalf("首次建档失败: %v", err)
		}
		if err := settings.InitDefaults(ctx, u.ID); err != nil {
			t.Fatalf("重复建档应幂等: %v", err)
		}
	})

	t.Run("保存后能读回", func(t *testing.T) {
		s, _ := settings.Find(ctx, u.ID)
		s.LikeTagIDs = []uint64{3, 1, 2}
		s.DiggNoticeEnabled = false
		if err := settings.Save(ctx, s); err != nil {
			t.Fatalf("保存失败: %v", err)
		}
		got, _ := settings.Find(ctx, u.ID)
		if len(got.LikeTagIDs) != 3 || got.LikeTagIDs[0] != 3 {
			t.Fatalf("标签顺序应保持: %+v", got.LikeTagIDs)
		}
		if got.DiggNoticeEnabled {
			t.Fatal("通知开关未关闭")
		}
	})

	t.Run("统计按增量累加", func(t *testing.T) {
		if err := stats.Add(ctx, u.ID, domain.StatsDelta{Article: 2, View: 3}); err != nil {
			t.Fatalf("累加失败: %v", err)
		}
		if err := stats.Add(ctx, u.ID, domain.StatsDelta{Article: 1}); err != nil {
			t.Fatalf("累加失败: %v", err)
		}
		got, _ := stats.Find(ctx, u.ID)
		if got.ArticleCount != 3 || got.ViewCount != 3 {
			t.Fatalf("累加结果不对: %+v", got)
		}
	})
}

func TestLoadDisplays(t *testing.T) {
	t.Parallel()
	e := setup(t)
	repo, ctx := e.repo, e.ctx
	u := newUser("frank")
	if err := repo.Create(ctx, &u); err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	got, err := repo.LoadDisplays(ctx, []domain.UserID{u.ID, u.ID, 999999})
	if err != nil {
		t.Fatalf("批量读取失败: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("去重后应只有 1 条: %d", len(got))
	}
	if got[u.ID].Nickname != "昵称-frank" {
		t.Fatalf("昵称不对: %+v", got[u.ID])
	}
}

func TestSyncDisplay(t *testing.T) {
	t.Parallel()
	e := setup(t)
	repo, ctx := e.repo, e.ctx
	u := newUser("grace")
	if err := repo.Create(ctx, &u); err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// 先造一条评论，它的冗余昵称是旧值。
	const commentID = 424242
	if err := e.db.Exec(`INSERT INTO comment_models (id, user_id, user_nickname, user_avatar) VALUES (?, ?, ?, ?)`,
		commentID, u.ID.Uint64(), "旧昵称", "old.png").Error; err != nil {
		t.Fatalf("造评论失败: %v", err)
	}

	nick := "改名后"
	if err := repo.UpdateProfile(ctx, u.ID, domain.ProfileUpdate{Nickname: &nick}, nil); err != nil {
		t.Fatalf("更新昵称失败: %v", err)
	}
	if err := repo.SyncDisplay(ctx, u.ID); err != nil {
		t.Fatalf("同步快照失败: %v", err)
	}

	var afterNickname string
	if err := e.db.Raw(`SELECT user_nickname FROM comment_models WHERE id = ?`, commentID).
		Scan(&afterNickname).Error; err != nil {
		t.Fatalf("回查评论失败: %v", err)
	}
	if afterNickname != "改名后" {
		t.Fatalf("评论里的昵称未刷新: %q", afterNickname)
	}
}
