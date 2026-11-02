package apptest

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"myblogx/internal/kernel/enum/image_ref_enum"
	"myblogx/internal/media/domain"
)

// OrphanStore 是 domain.OrphanStore 的内存替身：图片表 + 引用账本 + 真实引用。
//
// 图片行直接复用 Repo，而不是另存一份：清单读的就是图片表，"扫描没动
// 图片表"这条只读断言必须打在同一个对象上才有意义。
type OrphanStore struct {
	*Repo

	// refs 是引用账本（image_ref_models）的内存形态：一张图挂了哪些引用行。
	refs map[domain.ImageID][]image_ref_enum.RefField
	// live 是重扫四类业务表得到的真实引用数，按 object_key 记。
	live map[string]int

	failures map[string]error
	calls    map[string]int
}

// NewOrphanStore 构造一个空的清单数据源。
func NewOrphanStore() *OrphanStore {
	return &OrphanStore{
		Repo:     NewRepo(),
		refs:     map[domain.ImageID][]image_ref_enum.RefField{},
		live:     map[string]int{},
		failures: map[string]error{},
		calls:    map[string]int{},
	}
}

// AddLedgerRef 在账本里追加一条引用行，相当于 CDC 记下的一次引用。
func (s *OrphanStore) AddLedgerRef(id domain.ImageID, field image_ref_enum.RefField) {
	s.refs[id] = append(s.refs[id], field)
}

// SetLiveRefs 设置某个 object_key 的真实引用数。
func (s *OrphanStore) SetLiveRefs(objectKey string, count int) {
	s.live[objectKey] = count
}

// FailNext 让下一次 method 调用返回 err（故障注入）。
func (s *OrphanStore) FailNext(method string, err error) {
	s.failures[method] = err
}

// Calls 返回 method 的累计调用次数。
func (s *OrphanStore) Calls(method string) int { return s.calls[method] }

// Snapshot 把三份事实序列化成一个可比较的字符串。
//
// 只读用例在扫描前后各取一次：两次相同说明图片表、账本、真实引用都没被写过。
func (s *OrphanStore) Snapshot() string {
	images := make([]domain.Image, 0, len(s.byID))
	for _, img := range s.byID {
		images = append(images, img)
	}
	sort.Slice(images, func(i, j int) bool {
		return images[i].ID.Uint64() < images[j].ID.Uint64()
	})

	var b strings.Builder
	for _, img := range images {
		fmt.Fprintf(&b, "image|%s|%s|%s|%d|%s\n",
			img.ID, img.ObjectKey, img.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), img.Status, img.Hash)
		for index, field := range s.refs[img.ID] {
			fmt.Fprintf(&b, "ledger|%s|%d|%s\n", img.ID, index, field)
		}
	}
	keys := make([]string, 0, len(s.live))
	for key := range s.live {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&b, "live|%s|%d\n", key, s.live[key])
	}
	return b.String()
}

// ListCandidates 实现 domain.OrphanStore。
func (s *OrphanStore) ListCandidates(_ context.Context, spec domain.OrphanScanSpec) ([]domain.Image, int64, error) {
	if err := s.hit("ListCandidates"); err != nil {
		return nil, 0, err
	}

	eligible := make([]domain.Image, 0, len(s.byID))
	for _, img := range s.byID {
		if !statusAllowed(spec.Statuses, img.Status) {
			continue
		}
		if !img.CreatedAt.Before(spec.UploadedBefore) {
			continue
		}
		eligible = append(eligible, img)
	}
	sort.Slice(eligible, func(i, j int) bool {
		if !eligible[i].CreatedAt.Equal(eligible[j].CreatedAt) {
			return eligible[i].CreatedAt.Before(eligible[j].CreatedAt)
		}
		return eligible[i].ID.Uint64() < eligible[j].ID.Uint64()
	})

	total := int64(len(eligible))
	limit := spec.Limit
	if limit <= 0 || limit > len(eligible) {
		limit = len(eligible)
	}
	return eligible[:limit], total, nil
}

// LedgerSnapshot 实现 domain.OrphanStore。
func (s *OrphanStore) LedgerSnapshot(_ context.Context, ids []domain.ImageID) (domain.LedgerSnapshot, error) {
	if err := s.hit("LedgerSnapshot"); err != nil {
		return domain.LedgerSnapshot{}, err
	}
	out := domain.LedgerSnapshot{
		Refs:    make(map[domain.ImageID]int, len(ids)),
		Avatars: make(map[domain.ImageID]bool, len(ids)),
	}
	for _, id := range ids {
		fields := s.refs[id]
		out.Refs[id] = len(fields)
		for _, field := range fields {
			if field == image_ref_enum.RefFieldUserAvatar {
				out.Avatars[id] = true
			}
		}
	}
	return out, nil
}

// LiveRefs 实现 domain.OrphanStore。
func (s *OrphanStore) LiveRefs(_ context.Context, objectKeys []string) (map[string]int, error) {
	if err := s.hit("LiveRefs"); err != nil {
		return nil, err
	}
	out := make(map[string]int, len(objectKeys))
	for _, key := range objectKeys {
		if count := s.live[key]; count > 0 {
			out[key] = count
		}
	}
	return out, nil
}

// hit 记一次调用并消费注入的故障。
func (s *OrphanStore) hit(method string) error {
	s.calls[method]++
	if err := s.failures[method]; err != nil {
		delete(s.failures, method)
		return err
	}
	return nil
}

func statusAllowed(statuses []domain.Status, status domain.Status) bool {
	for _, allowed := range statuses {
		if allowed == status {
			return true
		}
	}
	return false
}

// CountingStorage 在 Storage 之上记账：记下 Delete 调用次数，并可注入删除失败。
//
// 它存在的唯一目的，是让"只读清单绝不碰存储"这条断言有地方落：
// 用例把失败注入打开，扫描结束后仍要求删除次数为 0。
type CountingStorage struct {
	mu          sync.Mutex
	deleteCalls int
	failDelete  error
}

// NewCountingStorage 构造。
func NewCountingStorage() *CountingStorage { return &CountingStorage{} }

// FailNextDelete 让下一次 Delete 返回 err。
func (s *CountingStorage) FailNextDelete(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failDelete = err
}

// DeleteCalls 返回 Delete 的累计调用次数。
func (s *CountingStorage) DeleteCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deleteCalls
}

// Token 实现 domain.ObjectStorage。
func (s *CountingStorage) Token(p domain.Policy) (domain.Token, error) {
	return Storage{}.Token(p)
}

// VerifyCallback 实现 domain.ObjectStorage。
func (s *CountingStorage) VerifyCallback(req *http.Request) (bool, error) {
	return Storage{}.VerifyCallback(req)
}

// Stat 实现 domain.ObjectStorage。
func (s *CountingStorage) Stat(bucket, key string) (domain.ObjectInfo, error) {
	return Storage{}.Stat(bucket, key)
}

// Delete 实现 domain.ObjectStorage。
func (s *CountingStorage) Delete(bucket, key string) error {
	s.mu.Lock()
	s.deleteCalls++
	failed := s.failDelete
	s.failDelete = nil
	s.mu.Unlock()
	if failed != nil {
		return failed
	}
	return Storage{}.Delete(bucket, key)
}

// ImageInfo 实现 domain.ObjectStorage。
func (s *CountingStorage) ImageInfo(key string) (domain.ImageInfo, error) {
	return Storage{}.ImageInfo(key)
}

// ObjectURL 实现 domain.ObjectStorage。
func (s *CountingStorage) ObjectURL(key string) string { return Storage{}.ObjectURL(key) }

var _ domain.ObjectStorage = (*CountingStorage)(nil)

// Recorder 记录日志调用，供"结论写进日志"的用例断言。
type Recorder struct {
	mu      sync.Mutex
	entries []string
}

// NewRecorder 构造。
func NewRecorder() *Recorder { return &Recorder{} }

// Infof 实现 domain.Logger。
func (r *Recorder) Infof(format string, args ...any) { r.append("info", format, args...) }

// Warnf 实现 domain.Logger。
func (r *Recorder) Warnf(format string, args ...any) { r.append("warn", format, args...) }

// Errorf 实现 domain.Logger。
func (r *Recorder) Errorf(format string, args ...any) { r.append("error", format, args...) }

// All 返回全部日志（"级别 内容"）。
func (r *Recorder) All() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.entries))
	copy(out, r.entries)
	return out
}

// Last 返回最后一条日志；没有日志时返回空串。
func (r *Recorder) Last() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) == 0 {
		return ""
	}
	return r.entries[len(r.entries)-1]
}

func (r *Recorder) append(level, format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, level+" "+fmt.Sprintf(format, args...))
}

var _ domain.Logger = (*Recorder)(nil)
