package app

import (
	"context"

	"myblogx/internal/apperr"
	sitedomain "myblogx/internal/site/domain"
)

// validateArticleCrop 校验文章封面取景是否在取值范围内；nil 表示调用方没传。
//
// 口径与轮播封面同一套（焦点 0–1、缩放 1–2），但错误码用文章自己的：
// 排查时从码就能看出被拒的是文章封面还是轮播封面。用错误而不是钳制——
// 钳制会让作者看到的取景与读者看到的取景分家。
func validateArticleCrop(crop *sitedomain.Crop) error {
	if crop == nil {
		return nil
	}
	inRange := crop.FocusX >= sitedomain.MinFocus && crop.FocusX <= sitedomain.MaxFocus &&
		crop.FocusY >= sitedomain.MinFocus && crop.FocusY <= sitedomain.MaxFocus &&
		crop.Zoom >= sitedomain.MinZoom && crop.Zoom <= sitedomain.MaxZoom
	if !inRange {
		return apperr.New(apperr.CodeArticleCropInvalid)
	}
	return nil
}

// upsertArticleCrop 把封面取景写进通用取景表；crop 为 nil 表示没提交，不写。
//
// 与文章本体同事务：只落文章不落取景，读者侧会按"居中、缩放 1"渲染，
// 作者以为取景生效了。删除文章时对应的行由 deleteArticleCrop 一并清掉。
func upsertArticleCrop(ctx context.Context, r Repos, articleID uint64, crop *sitedomain.Crop) error {
	if crop == nil {
		return nil
	}
	return r.Crops().Upsert(ctx, sitedomain.CropTargetArticle, articleID, *crop)
}

// loadArticleCrop 读回一篇文章的封面取景；没有记录时退化成居中、缩放 1。
//
// "没有记录"不是错误：取景表是后加的，存量文章本来就没有行；
// 渲染必须与引入取景之前完全一致。
func loadArticleCrop(ctx context.Context, r Repos, articleID uint64) (sitedomain.Crop, error) {
	crops, err := r.Crops().Load(ctx, sitedomain.CropTargetArticle, []uint64{articleID})
	if err != nil {
		return sitedomain.Crop{}, err
	}
	if crop, ok := crops[articleID]; ok {
		return crop, nil
	}
	return sitedomain.DefaultCrop, nil
}

// loadArticleCrops 批量读回封面取景，缺记录的一律补缺省。
func loadArticleCrops(ctx context.Context, r Repos, articleIDs []uint64) (map[uint64]sitedomain.Crop, error) {
	crops, err := r.Crops().Load(ctx, sitedomain.CropTargetArticle, articleIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[uint64]sitedomain.Crop, len(articleIDs))
	for _, id := range articleIDs {
		if crop, ok := crops[id]; ok {
			out[id] = crop
			continue
		}
		out[id] = sitedomain.DefaultCrop
	}
	return out, nil
}

// deleteArticleCrop 真删一篇文章的取景行（取景表没有软删列）。
func deleteArticleCrop(ctx context.Context, r Repos, articleID uint64) error {
	return r.Crops().Delete(ctx, sitedomain.CropTargetArticle, []uint64{articleID})
}
