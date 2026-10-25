package enum

import (
	"strings"
)

type ImageProvider string

const (
	ImageProviderQiNiu ImageProvider = "qiniu"
)

type ImageUploadTaskStatus string

const (
	ImageUploadTaskPending ImageUploadTaskStatus = "pending"
	ImageUploadTaskReady   ImageUploadTaskStatus = "ready"
	ImageUploadTaskFailed  ImageUploadTaskStatus = "failed"
)

type ImageStatus uint

const (
	ImageStatusUnknown ImageStatus = iota
	ImageStatusPass
	ImageStatusDeleted
	ImageStatusOrphaned
	ImageStatusReviewing
	ImageStatusBlocked
)

func (s ImageStatus) String() string {
	switch s {
	case ImageStatusPass:
		return "pass"
	case ImageStatusDeleted:
		return "deleted"
	case ImageStatusOrphaned:
		return "orphaned"
	case ImageStatusReviewing:
		return "review"
	case ImageStatusBlocked:
		return "block"
	default:
		return "unknown"
	}
}

// 将七牛的审核建议映射为系统图片状态
func ImageStatusMapString(suggestion string) ImageStatus {
	switch strings.ToLower(strings.TrimSpace(suggestion)) {
	case "pass":
		return ImageStatusPass
	case "review":
		return ImageStatusReviewing
	case "block":
		return ImageStatusBlocked
	case "deleted":
		return ImageStatusDeleted
	case "orphaned":
		return ImageStatusOrphaned
	default:
		return ImageStatusUnknown
	}
}
