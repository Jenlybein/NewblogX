package image_service

import (
	"time"

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
)

type UploadPolicy struct {
	Bucket      string
	ObjectKey   string
	CallbackURL string
	ExpireAt    time.Time
	MaxSize     int64
	EndUser     string
}

type UploadTokenResult struct {
	Token     string
	Bucket    string
	ObjectKey string
	ExpireAt  time.Time
}

type ImageInfoResult struct {
	Format string `json:"format"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Size   int64  `json:"size"`
}

type ImageUploadTask struct {
	ID           ctype.ID                   `json:"id"`
	UserID       ctype.ID                   `json:"user_id"`
	Provider     enum.ImageProvider         `json:"provider"`
	Status       enum.ImageUploadTaskStatus `json:"status"`
	Bucket       string                     `json:"bucket"`
	ObjectKey    string                     `json:"object_key"`
	OriginalName string                     `json:"original_name"`
	DeclaredMime string                     `json:"declared_mime"`
	DeclaredSize int64                      `json:"declared_size"`
	VerifiedMime string                     `json:"verified_mime"`
	VerifiedSize int64                      `json:"verified_size"`
	Width        int                        `json:"width"`
	Height       int                        `json:"height"`
	Hash         string                     `json:"hash"`
	ErrorMsg     string                     `json:"error_msg"`
	ExpiresAt    time.Time                  `json:"expires_at"`
	ConfirmedAt  *time.Time                 `json:"confirmed_at"`
	ImageID      *ctype.ID                  `json:"image_id"`
	ImageURL     string                     `json:"image_url"`
}

type CreateUploadTaskResult struct {
	Task       *ImageUploadTask
	UploadInfo *UploadTokenResult
	Image      *models.ImageModel
	SkipUpload bool
}

type ConfirmUploadTaskResult struct {
	Task  *ImageUploadTask
	Image *models.ImageModel
}

type verifiedImage struct {
	TaskID             ctype.ID
	UserID             ctype.ID
	Bucket             string
	ObjectKey          string
	FileName           string
	Hash               string
	MimeType           string
	Size               int64
	Width              int
	Height             int
	ShouldDeleteUpload bool
}

type uploadedObjectMeta struct {
	Bucket string
	Hash   string
	Size   int64
}

type qiniuAuditCallbackPayload struct {
	InputKey   string                   `json:"inputKey"`
	Key        string                   `json:"key"`
	ObjectKey  string                   `json:"objectKey"`
	Suggestion string                   `json:"suggestion"`
	Result     qiniuAuditResultLevel1   `json:"result"`
	Items      []qiniuAuditCallbackItem `json:"items"`
}

type qiniuAuditCallbackItem struct {
	Result qiniuAuditResultLevel1 `json:"result"`
}

type qiniuAuditResultLevel1 struct {
	Suggestion string                 `json:"suggestion"`
	Result     qiniuAuditResultLevel2 `json:"result"`
}

type qiniuAuditResultLevel2 struct {
	Suggestion string `json:"suggestion"`
}
