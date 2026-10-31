// Package config 维护站点的"运行时配置"：数据库里的一份分段 JSON，进程内缓存。
//
// 与 conf 包的分工：conf 是启动时从 settings.yaml 读到的基线配置，
// 本包是管理员在后台改过之后、覆盖在基线之上的那份（落 runtime_config_models 表）。
// 两者结构相同，读取一律走本包，保证"改完立刻生效"。
//
// 存储形态是"一段一行"：每段一个 name、一段完整 JSON。这样加一个配置段
// 不需要改表结构 —— 而项目的 -db 只执行 CREATE TABLE IF NOT EXISTS、不做
// ALTER，列化会让每次加字段都变成 DDL 变更。
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
	"myblogx/internal/apperr"
	"myblogx/utils/envyaml"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// 各配置段在表里的 name。
const (
	SectionSite    = "site"
	SectionAI      = "ai"
	SectionMail    = "mail"
	SectionStorage = "storage"
	SectionOAuth   = "oauth"
	SectionLog     = "log"
	SectionUpload  = "upload"
	SectionDeco    = "deco"
)

// defaultSettingsFileName 是默认值文件名；与 settings.yaml 同目录。
const defaultSettingsFileName = "site_default_settings.yaml"

// RuntimeConfig 是一份完整的运行时配置。
type RuntimeConfig struct {
	Site    conf.Site
	AI      conf.AI
	Mail    conf.Mail
	Storage conf.QiNiu
	OAuth   conf.QQ
	Log     conf.Logrus
	Upload  conf.Upload
	Deco    conf.Deco
}

// RuntimeConfigService 负责运行时配置的加载、缓存与更新。
//
// 该服务通过构造器显式注入，避免 package 级全局状态。
type RuntimeConfigService struct {
	defaults conf.RuntimeDefaults
	logger   *logrus.Logger
	db       *gorm.DB

	// logApplier 在日志配置被改动后调用，用来把级别真正下发给 logger。
	//
	// 做成回调而不是直接调 core：平台包不能依赖 core（core 依赖本包，会成环）。
	// 也不能自己 SetLevel —— logger 实例是别人的，本包只是存配置。
	logApplier func(conf.Logrus)

	mu    sync.RWMutex
	cache *RuntimeConfig
}

// NewRuntimeConfigService 构造服务。
//
// defaults 是各段默认值：数据库里缺段、或缺字段时用它补齐。
func NewRuntimeConfigService(defaults conf.RuntimeDefaults, logger *logrus.Logger, db *gorm.DB) *RuntimeConfigService {
	return &RuntimeConfigService{
		defaults: defaults,
		logger:   logger,
		db:       db,
	}
}

// SetLogApplier 注册日志配置的下发回调。
//
// 装配层在构造后立刻调用；不注册时 UpdateLog 只落库 + 刷缓存，
// 日志级别要等重启才对新的日志语句生效。
func (s *RuntimeConfigService) SetLogApplier(fn func(conf.Logrus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logApplier = fn
}

// LoadDefaults 从 site_default_settings.yaml 读业务配置默认值。
//
// 只读业务段（site / ai / mail / storage / oauth / upload / deco）；日志默认值
// 属于进程级配置，来自 settings.yaml，由装配层填进 RuntimeDefaults.Log。
func LoadDefaults(configFile string) (conf.RuntimeDefaults, error) {
	var out conf.RuntimeDefaults
	if configFile == "" {
		return out, apperr.New(apperr.CodeRuntimeConfigPathMissing)
	}
	file := filepath.Join(filepath.Dir(configFile), defaultSettingsFileName)
	byteData, err := os.ReadFile(file)
	if err != nil {
		return out, err
	}
	if err := envyaml.Unmarshal(byteData, &out); err != nil {
		return out, apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
	}
	return out, nil
}

// ── 读取 ──────────────────────────────────────────────────────

// GetRuntimeConfig 返回当前生效的完整配置。
func (s *RuntimeConfigService) GetRuntimeConfig() RuntimeConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.snapshotLocked()
}

// GetRuntimeSite 返回站点配置。
func (s *RuntimeConfigService) GetRuntimeSite() conf.Site {
	return s.GetRuntimeConfig().Site
}

// GetRuntimeLogin 返回登录方式开关。
func (s *RuntimeConfigService) GetRuntimeLogin() siteconf.Login {
	return s.GetRuntimeSite().Login
}

// GetRuntimeArticle 返回文章相关开关。
func (s *RuntimeConfigService) GetRuntimeArticle() siteconf.Article {
	return s.GetRuntimeSite().Article
}

// GetRuntimeComment 返回评论相关开关。
func (s *RuntimeConfigService) GetRuntimeComment() siteconf.Comment {
	return s.GetRuntimeSite().Comment
}

// GetRuntimeAI 返回 AI 配置。
func (s *RuntimeConfigService) GetRuntimeAI() conf.AI {
	return s.GetRuntimeConfig().AI
}

// GetRuntimeMail 返回邮件（腾讯云 SES）配置。
func (s *RuntimeConfigService) GetRuntimeMail() conf.Mail {
	return s.GetRuntimeConfig().Mail
}

// GetRuntimeStorage 返回对象存储配置。
func (s *RuntimeConfigService) GetRuntimeStorage() conf.QiNiu {
	return s.GetRuntimeConfig().Storage
}

// GetRuntimeOAuth 返回第三方登录配置。
func (s *RuntimeConfigService) GetRuntimeOAuth() conf.QQ {
	return s.GetRuntimeConfig().OAuth
}

// GetRuntimeLog 返回日志配置。
func (s *RuntimeConfigService) GetRuntimeLog() conf.Logrus {
	return s.GetRuntimeConfig().Log
}

// GetRuntimeUpload 返回上传策略。
func (s *RuntimeConfigService) GetRuntimeUpload() conf.Upload {
	return s.GetRuntimeConfig().Upload
}

// GetRuntimeDeco 返回装饰配置。
func (s *RuntimeConfigService) GetRuntimeDeco() conf.Deco {
	return s.GetRuntimeConfig().Deco
}

// snapshotLocked 返回缓存的一份副本；缓存为空时回退到默认值。
//
// 返回副本而不是指针：调用方拼装响应时可能改字段（例如把密钥换成占位符），
// 直接给出缓存指针会让"读"变成"写"。
func (s *RuntimeConfigService) snapshotLocked() *RuntimeConfig {
	if s.cache != nil {
		cfg := *s.cache
		return &cfg
	}
	return &RuntimeConfig{
		Site:    s.defaults.Site,
		AI:      s.defaults.AI,
		Mail:    s.defaults.Mail,
		Storage: s.defaults.Storage,
		OAuth:   s.defaults.OAuth,
		Log:     s.defaults.Log,
		Upload:  s.defaults.Upload,
		Deco:    s.defaults.Deco,
	}
}

// ── 加载 ──────────────────────────────────────────────────────

// InitRuntimeConfig 会在启动时把运行时配置从数据库加载到内存。
// 数据库里缺某个段时，会用该段的默认值补一行进去。
func (s *RuntimeConfigService) InitRuntimeConfig() error {
	if s.db == nil {
		return apperr.New(apperr.CodeRuntimeConfigDBMissing)
	}
	if !s.db.Migrator().HasTable(&runtimeConfigRow{}) {
		return apperr.New(apperr.CodeRuntimeConfigTableMissing)
	}

	var loaded *RuntimeConfig
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		payloads, err := loadPayloads(tx)
		if err != nil {
			return err
		}
		loaded = s.decodeAll(payloads)
		return s.seedMissing(tx, payloads)
	}); err != nil {
		return err
	}

	s.setCache(loaded)
	return nil
}

// loadPayloads 读出所有段，返回 name → payload。
//
// 只取未软删除的行：软删除在这里的语义是"这段配置作废、回到默认值"。
// 对应的行随后会被 seedMissing 恢复成默认值，不会一直挂在库里。
func loadPayloads(tx *gorm.DB) (map[string]string, error) {
	var rows []runtimeConfigRow
	if err := tx.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Name] = row.Payload
	}
	return out, nil
}

// decodeAll 把库里读到的各段合并默认值后解码成强类型。
//
// 单段解码失败不让整个进程起不来：记一条 warn 并按默认值处理那一段。
// 理由是"配置坏了"和"数据库连不上"的严重性不同 —— 前者应该降级可用，
// 后者才该拦住启动。
func (s *RuntimeConfigService) decodeAll(payloads map[string]string) *RuntimeConfig {
	return &RuntimeConfig{
		Site:    decodeOrDefault(SectionSite, s.defaults.Site, payloads, s.logger),
		AI:      decodeOrDefault(SectionAI, s.defaults.AI, payloads, s.logger),
		Mail:    decodeOrDefault(SectionMail, s.defaults.Mail, payloads, s.logger),
		Storage: decodeOrDefault(SectionStorage, s.defaults.Storage, payloads, s.logger),
		OAuth:   decodeOrDefault(SectionOAuth, s.defaults.OAuth, payloads, s.logger),
		Log:     decodeOrDefault(SectionLog, s.defaults.Log, payloads, s.logger),
		Upload:  decodeOrDefault(SectionUpload, s.defaults.Upload, payloads, s.logger),
		Deco:    decodeOrDefault(SectionDeco, s.defaults.Deco, payloads, s.logger),
	}
}

// decodeOrDefault 解析单段，失败时记 warn 并回退该段默认值。
func decodeOrDefault[T any](name string, defaults T, payloads map[string]string, logger *logrus.Logger) T {
	out, err := decodeSection(name, defaults, payloads)
	if err != nil {
		if logger != nil {
			logger.Warnf("运行时配置：段 %s 解析失败，回退默认值: %v", name, err)
		}
		return defaults
	}
	return out
}

// decodeSection 把某段的默认值与库里存的 JSON 合并后解码。
//
// 合并是按字段做的：库里那段 JSON 少哪个字段就用默认值补哪个。
// 所以"新增一个配置字段"不需要写数据迁移，也不需要给老行补值。
func decodeSection[T any](name string, defaults T, payloads map[string]string) (T, error) {
	var out T
	merged, err := mergeJSON(defaults, payloads[name])
	if err != nil {
		return out, apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return out, apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
	}
	return out, nil
}

// seedMissing 给库里没有的段补一行默认值。
//
// 走 upsertPayload 而不是直接 Create：本表在 name 上有唯一索引，若某段曾被
// 软删除（行还在、只是 deleted_at 非空），直接 Create 会撞索引让启动失败。
// upsertPayload 会把软删除行恢复并用默认值覆盖，这正是想要的语义。
func (s *RuntimeConfigService) seedMissing(tx *gorm.DB, payloads map[string]string) error {
	seeds := []struct {
		name  string
		value any
	}{
		{SectionSite, s.defaults.Site},
		{SectionAI, s.defaults.AI},
		{SectionMail, s.defaults.Mail},
		{SectionStorage, s.defaults.Storage},
		{SectionOAuth, s.defaults.OAuth},
		{SectionLog, s.defaults.Log},
		{SectionUpload, s.defaults.Upload},
		{SectionDeco, s.defaults.Deco},
	}
	for _, seed := range seeds {
		if _, ok := payloads[seed.name]; ok {
			continue
		}
		raw, err := json.Marshal(seed.value)
		if err != nil {
			return apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
		}
		if err := upsertPayload(tx, seed.name, string(raw)); err != nil {
			return err
		}
	}
	return nil
}

func (s *RuntimeConfigService) setCache(cfg *RuntimeConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cfg == nil {
		s.cache = nil
		return
	}
	cp := *cfg
	s.cache = &cp
}

// ── 更新 ──────────────────────────────────────────────────────

// UpdateSite 覆盖站点配置并刷新缓存。
func (s *RuntimeConfigService) UpdateSite(site conf.Site) error {
	return updateSection(s, SectionSite, site)
}

// UpdateAI 覆盖 AI 配置并刷新缓存。
func (s *RuntimeConfigService) UpdateAI(ai conf.AI) error {
	return updateSection(s, SectionAI, ai)
}

// UpdateMail 覆盖邮件配置并刷新缓存。
func (s *RuntimeConfigService) UpdateMail(mail conf.Mail) error {
	return updateSection(s, SectionMail, mail)
}

// UpdateStorage 覆盖对象存储配置并刷新缓存。
func (s *RuntimeConfigService) UpdateStorage(storage conf.QiNiu) error {
	return updateSection(s, SectionStorage, storage)
}

// UpdateOAuth 覆盖第三方登录配置并刷新缓存。
func (s *RuntimeConfigService) UpdateOAuth(oauth conf.QQ) error {
	return updateSection(s, SectionOAuth, oauth)
}

// UpdateLog 覆盖日志配置并刷新缓存，并把级别下发给 logger。
//
// 本包不持有 logger，所以下发通过构造时注册的回调完成（见 SetLogApplier）。
func (s *RuntimeConfigService) UpdateLog(log conf.Logrus) error {
	if err := updateSection(s, SectionLog, log); err != nil {
		return err
	}
	s.mu.RLock()
	apply := s.logApplier
	s.mu.RUnlock()
	if apply != nil {
		apply(log)
	}
	return nil
}

// UpdateUpload 覆盖上传策略并刷新缓存。
func (s *RuntimeConfigService) UpdateUpload(upload conf.Upload) error {
	return updateSection(s, SectionUpload, upload)
}

// UpdateDeco 覆盖装饰配置并刷新缓存。
func (s *RuntimeConfigService) UpdateDeco(deco conf.Deco) error {
	return updateSection(s, SectionDeco, deco)
}

// updateSection 是各段更新的公共骨架：事务里 upsert 单行，提交后刷内存缓存。
//
// 泛型写成包级函数而不是方法：Go 的方法不能自带类型参数。
func updateSection[T any](s *RuntimeConfigService, name string, value T) error {
	if s.db == nil {
		return apperr.New(apperr.CodeRuntimeConfigDBMissing)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return upsertPayload(tx, name, string(raw))
	}); err != nil {
		return err
	}
	s.applySection(name, value)
	return nil
}

// upsertPayload 按 name 更新或插入一行。
//
// 查询必须带 Unscoped：本表在 name 上有唯一索引，而 gorm 的软删除只是把
// deleted_at 填上、行还在表里。若用默认 scope 查就会"看不见"被软删除的行，
// 于是走 INSERT 分支、撞唯一索引，表现为后台保存永久报错。
// 查到软删除行时把它恢复（deleted_at 置空）并写入新值。
//
// 用 Find 而不是 Take：这段在启动时会对每个缺失的段各跑一次，
// Take 的 ErrRecordNotFound 会把 gorm 的 record not found 日志刷满。
func upsertPayload(tx *gorm.DB, name, payload string) error {
	var rows []runtimeConfigRow
	if err := tx.Unscoped().Where("name = ?", name).Limit(1).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return tx.Create(&runtimeConfigRow{Name: name, Payload: payload}).Error
	}

	row := rows[0]
	if row.DeletedAt.Valid {
		return tx.Unscoped().Model(&row).
			Updates(map[string]any{"payload": payload, "deleted_at": nil}).Error
	}
	return tx.Model(&row).Updates(map[string]any{"payload": payload}).Error
}

// applySection 把刚写入的值同步进缓存。
func (s *RuntimeConfigService) applySection(name string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.snapshotLocked()
	switch typed := value.(type) {
	case conf.Site:
		cfg.Site = typed
	case conf.AI:
		cfg.AI = typed
	case conf.Mail:
		cfg.Mail = typed
	case conf.QiNiu:
		cfg.Storage = typed
	case conf.QQ:
		cfg.OAuth = typed
	case conf.Logrus:
		cfg.Log = typed
	case conf.Upload:
		cfg.Upload = typed
	case conf.Deco:
		cfg.Deco = typed
	default:
		// 只有本包自己会调用 applySection，走到这里说明新增了段却忘了加分支。
		if s.logger != nil {
			s.logger.Warnf("运行时配置：未知的配置段 %s，缓存未更新", name)
		}
		return
	}
	s.cache = cfg
}
