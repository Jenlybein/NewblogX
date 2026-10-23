package image_service

import (
	"errors"
	"fmt"
	"github.com/qiniu/go-sdk/v7/storage"
)

func CreateUploadToken(deps Deps, policy UploadPolicy) (*UploadTokenResult, error) {
	if policy.Bucket == "" {
		return nil, errors.New("七牛 bucket 不能为空")
	}
	if policy.ObjectKey == "" {
		return nil, errors.New("七牛对象 key 不能为空")
	}
	if policy.MaxSize <= 0 {
		return nil, errors.New("七牛上传大小限制必须大于 0")
	}

	putPolicy := storage.PutPolicy{
		Scope:      fmt.Sprintf("%s:%s", policy.Bucket, policy.ObjectKey),
		Expires:    uint64(policy.ExpireAt.Unix()),
		InsertOnly: 1,
		EndUser:    policy.EndUser,
		FsizeLimit: policy.MaxSize,
		DetectMime: 1,
		MimeLimit:  "image/*",
		ReturnBody: `{"key":"$(key)","hash":"$(etag)","bucket":"$(bucket)","fsize":$(fsize)}`,
	}
	if policy.CallbackURL != "" {
		putPolicy.CallbackURL = policy.CallbackURL
		putPolicy.CallbackBody = `{"key":"$(key)","hash":"$(etag)","bucket":"$(bucket)","fsize":$(fsize)}`
		putPolicy.CallbackBodyType = "application/json"
	}

	return &UploadTokenResult{
		Token:     putPolicy.UploadToken(getQiniuRuntime(deps.QiNiu).mac),
		Bucket:    policy.Bucket,
		ObjectKey: policy.ObjectKey,
		ExpireAt:  policy.ExpireAt,
	}, nil
}
