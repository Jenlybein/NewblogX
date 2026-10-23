package image_api

import "myblogx/models/ctype"

type CreateImageUploadTaskRequest struct {
	FileName string `json:"file_name" binding:"required,max=255"`
	Size     int64  `json:"size" binding:"required"`
	MimeType string `json:"mime_type" binding:"required,max=128"`
	Hash     string `json:"hash" binding:"required,max=64"`
}

type CreateImageUploadTaskResponse struct {
	SkipUpload  bool     `json:"skip_upload"`
	UploadID    ctype.ID `json:"upload_id"`
	Provider    string   `json:"provider"`
	Bucket      string   `json:"bucket"`
	ObjectKey   string   `json:"object_key"`
	UploadToken string   `json:"upload_token"`
	Region      string   `json:"region"`
	ExpireAt    string   `json:"expire_at"`
	MaxSize     int64    `json:"max_size"`
	ImageID     ctype.ID `json:"image_id"`
	Status      string   `json:"status"`
	URL         string   `json:"url"`
	Hash        string   `json:"hash"`
}

type CompleteImageUploadTaskRequest struct {
	UploadID  ctype.ID `json:"upload_id" binding:"required"`
	ObjectKey string   `json:"object_key" binding:"required"`
}

type CompleteImageUploadTaskResponse struct {
	UploadID ctype.ID `json:"upload_id"`
	ImageID  ctype.ID `json:"image_id"`
	Status   string   `json:"status"`
	URL      string   `json:"url"`
	ErrorMsg string   `json:"error_msg,omitempty"`
}

type qiniuCallbackRequest struct {
	Key    string `json:"key"`
	Hash   string `json:"hash"`
	Bucket string `json:"bucket"`
	Fsize  int64  `json:"fsize"`
}

type UploadTaskStatusResponse struct {
	UploadID ctype.ID `json:"upload_id"`
	ImageID  ctype.ID `json:"image_id"`
	Status   string   `json:"status"`
	URL      string   `json:"url"`
	ErrorMsg string   `json:"error_msg,omitempty"`
	Hash     string   `json:"hash"`
}
