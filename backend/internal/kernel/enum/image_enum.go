package enum

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
