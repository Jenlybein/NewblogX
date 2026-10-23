package follow_service

import (
	"testing"

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum/relationship_enum"
	"myblogx/test/testutil"
)

type followUsers struct {
	owner     models.UserModel
	fansA     models.UserModel
	fansB     models.UserModel
	followedA models.UserModel
	outsider  models.UserModel
}

func TestCalUserRelationship(t *testing.T) {
	users := setupFollowEnv(t)

	if got := CalUserRelationship(testutil.DB(), users.owner.ID, users.outsider.ID); got != relationship_enum.RelationStranger {
		t.Fatalf("陌生人关系错误: %v", got)
	}

	createFollow(t, users.owner.ID, users.outsider.ID)
	if got := CalUserRelationship(testutil.DB(), users.outsider.ID, users.owner.ID); got != relationship_enum.RelationFans {
		t.Fatalf("被关注方向关系错误: %v", got)
	}
	if got := CalUserRelationship(testutil.DB(), users.owner.ID, users.outsider.ID); got != relationship_enum.RelationFollowed {
		t.Fatalf("关注方向关系错误: %v", got)
	}

	createFollow(t, users.outsider.ID, users.owner.ID)
	if got := CalUserRelationship(testutil.DB(), users.owner.ID, users.outsider.ID); got != relationship_enum.RelationFriend {
		t.Fatalf("互关关系错误: %v", got)
	}
}

func TestCalUserRelationshipReturnsStrangerForInvalidUser(t *testing.T) {
	users := setupFollowEnv(t)

	if got := CalUserRelationship(testutil.DB(), 0, users.owner.ID); got != relationship_enum.RelationStranger {
		t.Fatalf("A 为 0 时应为陌生人: %v", got)
	}
	if got := CalUserRelationship(testutil.DB(), users.owner.ID, 0); got != relationship_enum.RelationStranger {
		t.Fatalf("B 为 0 时应为陌生人: %v", got)
	}
	if got := CalUserRelationship(testutil.DB(), 0, 0); got != relationship_enum.RelationStranger {
		t.Fatalf("双方都为 0 时应为陌生人: %v", got)
	}
}

func TestCalUserRelationshipSelfDefaultsToStranger(t *testing.T) {
	users := setupFollowEnv(t)

	if got := CalUserRelationship(testutil.DB(), users.owner.ID, users.owner.ID); got != relationship_enum.RelationStranger {
		t.Fatalf("自己和自己当前应按陌生人处理: %v", got)
	}
}

func TestCalUserRelationshipBatch(t *testing.T) {
	users := setupFollowEnv(t)

	createFollow(t, users.owner.ID, users.fansA.ID)
	createFollow(t, users.fansB.ID, users.owner.ID)
	createFollow(t, users.owner.ID, users.followedA.ID)
	createFollow(t, users.followedA.ID, users.owner.ID)

	got := CalUserRelationshipBatch(testutil.DB(), users.owner.ID, []ctype.ID{
		users.fansA.ID,
		users.fansB.ID,
		users.followedA.ID,
		users.outsider.ID,
	})

	assertRelation(t, got, users.fansA.ID, relationship_enum.RelationFollowed)
	assertRelation(t, got, users.fansB.ID, relationship_enum.RelationFans)
	assertRelation(t, got, users.followedA.ID, relationship_enum.RelationFriend)
	assertRelation(t, got, users.outsider.ID, relationship_enum.RelationStranger)

	empty := CalUserRelationshipBatch(testutil.DB(), users.owner.ID, nil)
	if len(empty) != 0 {
		t.Fatalf("空列表应返回空 map: %+v", empty)
	}
}

func TestCalUserRelationshipBatchKeepsUnknownUsersAsStranger(t *testing.T) {
	users := setupFollowEnv(t)
	createFollow(t, users.owner.ID, users.followedA.ID)

	got := CalUserRelationshipBatch(testutil.DB(), users.owner.ID, []ctype.ID{
		users.followedA.ID,
		users.followedA.ID,
		users.outsider.ID,
		0,
	})

	assertRelation(t, got, users.followedA.ID, relationship_enum.RelationFollowed)
	assertRelation(t, got, users.outsider.ID, relationship_enum.RelationStranger)
	assertRelation(t, got, 0, relationship_enum.RelationStranger)
}

func setupFollowEnv(t *testing.T) followUsers {
	t.Helper()

	testutil.SetupSQLite(t, &models.UserModel{}, &models.UserConfModel{}, &models.UserFollowModel{})

	return followUsers{
		owner:     createFollowUser(t, "follow_owner"),
		fansA:     createFollowUser(t, "follow_fans_a"),
		fansB:     createFollowUser(t, "follow_fans_b"),
		followedA: createFollowUser(t, "followed_a"),
		outsider:  createFollowUser(t, "follow_outsider"),
	}
}

func createFollowUser(t *testing.T, username string) models.UserModel {
	t.Helper()

	user := models.UserModel{
		Username: username,
		Nickname: username + "_nick",
	}
	if err := testutil.DB().Create(&user).Error; err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	return user
}

func createFollow(t *testing.T, fansUserID, followedUserID ctype.ID) {
	t.Helper()

	row := models.UserFollowModel{
		FansUserID:     fansUserID,
		FollowedUserID: followedUserID,
	}
	if err := testutil.DB().Create(&row).Error; err != nil {
		t.Fatalf("创建关注关系失败 fans=%d followed=%d err=%v", fansUserID, followedUserID, err)
	}
}

func assertRelation(t *testing.T, relationMap map[ctype.ID]relationship_enum.Relation, userID ctype.ID, expect relationship_enum.Relation) {
	t.Helper()

	got, ok := relationMap[userID]
	if !ok {
		t.Fatalf("缺少用户 %d 的关系结果", userID)
	}
	if got != expect {
		t.Fatalf("用户 %d 关系错误: got=%v expect=%v", userID, got, expect)
	}
}
