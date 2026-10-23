package follow_api

import (
	"encoding/json"
	"myblogx/common"
	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum/relationship_enum"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type followResponse struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Msg  string          `json:"msg"`
}

type followListPayload struct {
	List    []FollowListResponse `json:"list"`
	HasMore bool                 `json:"has_more"`
}

type fansListPayload struct {
	List    []FansListResponse `json:"list"`
	HasMore bool               `json:"has_more"`
}

func newFollowAPI() FollowApi {
	return New(Deps{
		DB: testutil.DB(),
	})
}

func TestFollowAndUnfollowUserView(t *testing.T) {
	users := setupFollowEnv(t)
	api := newFollowAPI()

	t.Run("不能关注自己", func(t *testing.T) {
		c, w := newFollowCtx(t, http.MethodPost, users.owner, models.IDRequest{ID: users.owner.ID}, nil)
		api.FollowUserView(c)
		if readFollowCode(t, w) == 0 {
			t.Fatalf("关注自己应失败, body=%s", w.Body.String())
		}
	})

	t.Run("不能关注不存在的用户", func(t *testing.T) {
		c, w := newFollowCtx(t, http.MethodPost, users.owner, models.IDRequest{ID: ctype.ID(999999999999)}, nil)
		api.FollowUserView(c)
		if readFollowCode(t, w) == 0 {
			t.Fatalf("关注不存在用户应失败, body=%s", w.Body.String())
		}
	})

	t.Run("关注和取关流程", func(t *testing.T) {
		c1, w1 := newFollowCtx(t, http.MethodPost, users.owner, models.IDRequest{ID: users.followedA.ID}, nil)
		api.FollowUserView(c1)
		if readFollowCode(t, w1) != 0 {
			t.Fatalf("关注应成功, body=%s", w1.Body.String())
		}
		assertFollowCount(t, users.owner.ID, users.followedA.ID, 1)
		assertUserStatCounts(t, users.owner.ID, 0, 0, 1)
		assertUserStatCounts(t, users.followedA.ID, 0, 1, 0)

		c2, w2 := newFollowCtx(t, http.MethodPost, users.owner, models.IDRequest{ID: users.followedA.ID}, nil)
		api.FollowUserView(c2)
		if readFollowCode(t, w2) == 0 {
			t.Fatalf("重复关注应失败, body=%s", w2.Body.String())
		}

		c3, w3 := newFollowCtx(t, http.MethodDelete, users.owner, models.IDRequest{ID: users.owner.ID}, nil)
		api.UnfollowUserView(c3)
		if readFollowCode(t, w3) == 0 {
			t.Fatalf("取消关注自己应失败, body=%s", w3.Body.String())
		}

		c4, w4 := newFollowCtx(t, http.MethodDelete, users.owner, models.IDRequest{ID: users.followedA.ID}, nil)
		api.UnfollowUserView(c4)
		if readFollowCode(t, w4) != 0 {
			t.Fatalf("取消关注应成功, body=%s", w4.Body.String())
		}
		assertFollowCount(t, users.owner.ID, users.followedA.ID, 0)
		assertUserStatCounts(t, users.owner.ID, 0, 0, 0)
		assertUserStatCounts(t, users.followedA.ID, 0, 0, 0)

		c4b, w4b := newFollowCtx(t, http.MethodPost, users.owner, models.IDRequest{ID: users.followedA.ID}, nil)
		api.FollowUserView(c4b)
		if readFollowCode(t, w4b) != 0 {
			t.Fatalf("软删后重新关注应成功, body=%s", w4b.Body.String())
		}
		assertFollowCount(t, users.owner.ID, users.followedA.ID, 1)
		assertUserStatCounts(t, users.owner.ID, 0, 0, 1)
		assertUserStatCounts(t, users.followedA.ID, 0, 1, 0)

		c5, w5 := newFollowCtx(t, http.MethodDelete, users.owner, models.IDRequest{ID: users.followedA.ID}, nil)
		api.UnfollowUserView(c5)
		if readFollowCode(t, w5) != 0 {
			t.Fatalf("重新关注后的再次取消关注应成功, body=%s", w5.Body.String())
		}
		assertUserStatCounts(t, users.owner.ID, 0, 0, 0)
		assertUserStatCounts(t, users.followedA.ID, 0, 0, 0)

		c6, w6 := newFollowCtx(t, http.MethodDelete, users.owner, models.IDRequest{ID: users.followedA.ID}, nil)
		api.UnfollowUserView(c6)
		if readFollowCode(t, w6) == 0 {
			t.Fatalf("未关注时取消关注应失败, body=%s", w6.Body.String())
		}
	})
}

func TestFollowListView(t *testing.T) {
	users := setupFollowEnv(t)
	api := newFollowAPI()

	createFollowAt(t, users.owner.ID, users.followedA.ID, time.Now().Add(-2*time.Hour))
	createFollowAt(t, users.owner.ID, users.followedB.ID, time.Now().Add(-1*time.Hour))

	t.Run("查看自己的关注列表", func(t *testing.T) {
		c, w := newFollowCtx(t, http.MethodGet, users.owner, models.IDRequest{}, FollowListRequest{
			PageInfo: common.PageInfo{Page: 1, Limit: 10},
		})
		api.FollowListView(c)

		body := readFollowList(t, w)
		if body.Code != 0 {
			t.Fatalf("查询关注列表应成功, body=%s", w.Body.String())
		}
		if body.Data.HasMore {
			t.Fatalf("关注列表 has_more 应为 false")
		}
		if len(body.Data.List) != 2 {
			t.Fatalf("关注列表长度错误: %d", len(body.Data.List))
		}
		if body.Data.List[0].FollowedUserID != users.followedA.ID {
			t.Fatalf("关注列表首项异常: %+v", body.Data.List[0])
		}
		if int(body.Data.List[0].Relation) != int(relationship_enum.RelationFollowed) {
			t.Fatalf("关注列表关系字段异常: %+v", body.Data.List[0])
		}
		if body.Data.List[0].FollowTime.IsZero() {
			t.Fatalf("关注时间不应为空")
		}
	})

	t.Run("按关注对象过滤", func(t *testing.T) {
		c, w := newFollowCtx(t, http.MethodGet, users.owner, models.IDRequest{}, FollowListRequest{
			PageInfo:       common.PageInfo{Page: 1, Limit: 10},
			FollowedUserID: users.followedB.ID,
		})
		api.FollowListView(c)

		body := readFollowList(t, w)
		if body.Code != 0 {
			t.Fatalf("按关注对象过滤应成功, body=%s", w.Body.String())
		}
		if body.Data.HasMore {
			t.Fatalf("按关注对象过滤后 has_more 应为 false")
		}
		if len(body.Data.List) != 1 {
			t.Fatalf("过滤结果异常: %+v", body.Data)
		}
		if body.Data.List[0].FollowedUserID != users.followedB.ID {
			t.Fatalf("过滤对象异常: %+v", body.Data.List[0])
		}
	})

	t.Run("他人关注列表未公开", func(t *testing.T) {
		setFollowVisibility(t, users.owner.ID, false)

		c, w := newFollowCtx(t, http.MethodGet, users.outsider, models.IDRequest{}, FollowListRequest{
			PageInfo: common.PageInfo{Page: 1, Limit: 10},
			UserID:   users.owner.ID,
		})
		api.FollowListView(c)
		if readFollowCode(t, w) == 0 {
			t.Fatalf("未公开的关注列表应失败, body=%s", w.Body.String())
		}
	})
}

func TestFansListView(t *testing.T) {
	users := setupFollowEnv(t)
	api := newFollowAPI()

	createFollowAt(t, users.fansA.ID, users.owner.ID, time.Now().Add(-2*time.Hour))
	createFollowAt(t, users.fansB.ID, users.owner.ID, time.Now().Add(-1*time.Hour))

	t.Run("查看自己的粉丝列表", func(t *testing.T) {
		c, w := newFollowCtx(t, http.MethodGet, users.owner, models.IDRequest{}, FansListRequest{
			PageInfo: common.PageInfo{Page: 1, Limit: 10},
		})
		api.FansListView(c)

		body := readFansList(t, w)
		if body.Code != 0 {
			t.Fatalf("查询粉丝列表应成功, body=%s", w.Body.String())
		}
		if body.Data.HasMore {
			t.Fatalf("粉丝列表 has_more 应为 false")
		}
		if len(body.Data.List) != 2 {
			t.Fatalf("粉丝列表长度错误: %d", len(body.Data.List))
		}
		if body.Data.List[0].FansUserID != users.fansA.ID {
			t.Fatalf("粉丝列表首项异常: %+v", body.Data.List[0])
		}
		if int(body.Data.List[0].Relation) != int(relationship_enum.RelationFans) {
			t.Fatalf("粉丝列表关系字段异常: %+v", body.Data.List[0])
		}
		if body.Data.List[0].FollowTime.IsZero() {
			t.Fatalf("关注时间不应为空")
		}
	})

	t.Run("按粉丝过滤", func(t *testing.T) {
		c, w := newFollowCtx(t, http.MethodGet, users.owner, models.IDRequest{}, FansListRequest{
			PageInfo:   common.PageInfo{Page: 1, Limit: 10},
			FansUserID: users.fansB.ID,
		})
		api.FansListView(c)

		body := readFansList(t, w)
		if body.Code != 0 {
			t.Fatalf("按粉丝过滤应成功, body=%s", w.Body.String())
		}
		if body.Data.HasMore {
			t.Fatalf("按粉丝过滤后 has_more 应为 false")
		}
		if len(body.Data.List) != 1 {
			t.Fatalf("过滤结果异常: %+v", body.Data)
		}
		if body.Data.List[0].FansUserID != users.fansB.ID {
			t.Fatalf("过滤对象异常: %+v", body.Data.List[0])
		}
	})

	t.Run("他人粉丝列表未公开", func(t *testing.T) {
		setFansVisibility(t, users.owner.ID, false)

		c, w := newFollowCtx(t, http.MethodGet, users.outsider, models.IDRequest{}, FansListRequest{
			PageInfo: common.PageInfo{Page: 1, Limit: 10},
			UserID:   users.owner.ID,
		})
		api.FansListView(c)
		if readFollowCode(t, w) == 0 {
			t.Fatalf("未公开的粉丝列表应失败, body=%s", w.Body.String())
		}
	})
}

type followUsers struct {
	owner     models.UserModel
	outsider  models.UserModel
	fansA     models.UserModel
	fansB     models.UserModel
	followedA models.UserModel
	followedB models.UserModel
}

func setupFollowEnv(t *testing.T) followUsers {
	t.Helper()
	testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{}, &models.UserFollowModel{})
	testutil.SetConfig(&conf.Config{
		Jwt: conf.Jwt{
			Expire: 24,
			Secret: "follow-test-secret",
			Issuer: "blogx-test",
		},
	})

	return followUsers{
		owner:     createUser(t, "owner"),
		outsider:  createUser(t, "outsider"),
		fansA:     createUser(t, "fans_a"),
		fansB:     createUser(t, "fans_b"),
		followedA: createUser(t, "followed_a"),
		followedB: createUser(t, "followed_b"),
	}
}

func createUser(t *testing.T, name string) models.UserModel {
	t.Helper()
	user := models.UserModel{
		Username: name,
		Nickname: name + "_nick",
		Avatar:   name + ".png",
		Abstract: name + "_abstract",
	}
	if err := testutil.DB().Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败 name=%s err=%v", name, err)
	}
	return user
}

func createFollow(t *testing.T, fansUserID, followedUserID ctype.ID) {
	t.Helper()
	createFollowAt(t, fansUserID, followedUserID, time.Now())
}

func createFollowAt(t *testing.T, fansUserID, followedUserID ctype.ID, createdAt time.Time) {
	t.Helper()
	row := models.UserFollowModel{
		Model: models.Model{
			CreatedAt: createdAt,
		},
		FansUserID:     fansUserID,
		FollowedUserID: followedUserID,
	}
	if err := testutil.DB().Create(&row).Error; err != nil {
		t.Fatalf("创建关注关系失败 fans=%d followed=%d err=%v", fansUserID, followedUserID, err)
	}
}

func assertFollowCount(t *testing.T, fansUserID, followedUserID ctype.ID, expected int64) {
	t.Helper()
	var count int64
	if err := testutil.DB().Model(&models.UserFollowModel{}).
		Where("followed_user_id = ? and fans_user_id = ?", followedUserID, fansUserID).
		Count(&count).Error; err != nil {
		t.Fatalf("查询关注关系失败: %v", err)
	}
	if count != expected {
		t.Fatalf("关注关系数量错误: got=%d want=%d", count, expected)
	}
}

func assertUserStatCounts(t *testing.T, userID ctype.ID, viewCount, fansCount, followCount int) {
	t.Helper()
	var stat models.UserStatModel
	if err := testutil.DB().Take(&stat, "user_id = ?", userID).Error; err != nil {
		t.Fatalf("查询用户统计失败 user_id=%d err=%v", userID, err)
	}
	if stat.ViewCount != viewCount || stat.FansCount != fansCount || stat.FollowCount != followCount {
		t.Fatalf("用户统计异常 user_id=%d got=(view:%d fans:%d follow:%d) want=(view:%d fans:%d follow:%d)",
			userID, stat.ViewCount, stat.FansCount, stat.FollowCount, viewCount, fansCount, followCount)
	}
}

func setFollowVisibility(t *testing.T, userID ctype.ID, visible bool) {
	t.Helper()
	if err := testutil.DB().Model(&models.UserConfModel{}).
		Where("user_id = ?", userID).
		Update("follow_visibility", visible).Error; err != nil {
		t.Fatalf("更新关注可见性失败: %v", err)
	}
}

func setFansVisibility(t *testing.T, userID ctype.ID, visible bool) {
	t.Helper()
	if err := testutil.DB().Model(&models.UserConfModel{}).
		Where("user_id = ?", userID).
		Update("fans_visibility", visible).Error; err != nil {
		t.Fatalf("更新粉丝可见性失败: %v", err)
	}
}

func assertRelation(t *testing.T, got map[ctype.ID]relationship_enum.Relation, userID ctype.ID, expected relationship_enum.Relation) {
	t.Helper()
	if got[userID] != expected {
		t.Fatalf("关系错误 user=%d got=%v want=%v", userID, got[userID], expected)
	}
}

func newFollowCtx(t *testing.T, method string, user models.UserModel, uri models.IDRequest, query any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	token := testutil.IssueAccessToken(t, &user)

	req := httptest.NewRequest(method, "/follow", nil)
	req.Header.Set("token", token)
	c.Request = req

	if uri.ID != 0 {
		c.Set("requestUri", uri)
	}
	if query != nil {
		c.Set("requestQuery", query)
	}
	c.Set("claims", &jwts.MyClaims{
		Claims: jwts.Claims{
			UserID:   user.ID,
			Role:     user.Role,
			Username: user.Username,
		},
	})
	return c, w
}

func readFollowCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	return readFollowResponse(t, w).Code
}

func readFollowList(t *testing.T, w *httptest.ResponseRecorder) struct {
	Code int
	Data followListPayload
	Msg  string
} {
	t.Helper()
	resp := readFollowResponse(t, w)
	var payload followListPayload
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &payload); err != nil {
			t.Fatalf("解析关注列表失败: %v body=%s", err, w.Body.String())
		}
	}
	return struct {
		Code int
		Data followListPayload
		Msg  string
	}{
		Code: resp.Code,
		Data: payload,
		Msg:  resp.Msg,
	}
}

func readFansList(t *testing.T, w *httptest.ResponseRecorder) struct {
	Code int
	Data fansListPayload
	Msg  string
} {
	t.Helper()
	resp := readFollowResponse(t, w)
	var payload fansListPayload
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &payload); err != nil {
			t.Fatalf("解析粉丝列表失败: %v body=%s", err, w.Body.String())
		}
	}
	return struct {
		Code int
		Data fansListPayload
		Msg  string
	}{
		Code: resp.Code,
		Data: payload,
		Msg:  resp.Msg,
	}
}

func readFollowResponse(t *testing.T, w *httptest.ResponseRecorder) followResponse {
	t.Helper()

	var response followResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, w.Body.String())
	}
	return response
}
