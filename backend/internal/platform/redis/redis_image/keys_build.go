package redis_image

import "myblogx/models/ctype"

// uploadTaskIDKey 生成taskID对应的Redis Key
func uploadTaskIDKey(taskID ctype.ID) string {
	return "image:upload:task:id:" + taskID.String()
}

// uploadTaskObjectKey 生成objectKey对应的Redis Key
func uploadTaskObjectKey(objectKey string) string {
	return "image:upload:task:object:" + objectKey
}

// uploadTaskLockKey 生成任务锁对应的Redis Key
func uploadTaskLockKey(taskID ctype.ID) string {
	return "image:upload:task:lock:" + taskID.String()
}

// imageAuditKey 生成图片审核结果缓存 key。
func imageAuditKey(objectKey string) string {
	return "image:audit:" + objectKey
}
