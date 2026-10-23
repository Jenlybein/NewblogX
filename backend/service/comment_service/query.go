package comment_service

import "myblogx/repository/comment_repo"

type RootCommentItem = comment_repo.RootCommentItem
type ReplyCommentItem = comment_repo.ReplyCommentItem
type ManageCommentQuery = comment_repo.ManageCommentQuery
type ManageCommentItem = comment_repo.ManageCommentItem
type QueryService = comment_repo.QueryService

var NewQueryService = comment_repo.NewQueryService
