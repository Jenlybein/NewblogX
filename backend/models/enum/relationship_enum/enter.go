package relationship_enum

type Relation int8

/*
已关注——关注了对方，但是对方没有关注你
陌生人——双方都没有关注
粉丝——对方关注了你
好友——双方互关
*/

const (
	RelationStranger Relation = iota + 1
	RelationFollowed
	RelationFans
	RelationFriend
)
