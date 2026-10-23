package article_api

import "myblogx/models/ctype"

type ArticleAuthorInfoBindRequest struct {
	AuthorID ctype.ID `form:"author_id" binding:"required"`
}
