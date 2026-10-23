package follow_service

import (
	"myblogx/models/ctype"
	"myblogx/models/enum/relationship_enum"
	"myblogx/repository/follow_repo"

	"gorm.io/gorm"
)

// CalUserRelationship 计算用户关系。
func CalUserRelationship(db *gorm.DB, a, b ctype.ID) relationship_enum.Relation {
	return follow_repo.CalUserRelationship(db, a, b)
}

// CalUserRelationshipBatch 批量计算用户关系。
func CalUserRelationshipBatch(db *gorm.DB, user ctype.ID, userList []ctype.ID) map[ctype.ID]relationship_enum.Relation {
	return follow_repo.CalUserRelationshipBatch(db, user, userList)
}
