// Package config 维护站点的"运行时配置"：数据库里的一份 JSON，进程内缓存。
//
// 与 conf 包的分工：conf 是启动时从 settings.yaml 读到的基线配置，
// 本包是管理员在后台改过之后、覆盖在基线之上的那份（落 runtime_site_config_models 表）。
// 两者结构相同，读取一律走本包，保证"改完立刻生效"。
package config

import (
	"encoding/json"
	"errors"
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

// runtimeConfigName 是运行时配置在表里的唯一键。
const runtimeConfigName = "default"

// RuntimeConfig 是一份完整的运行时配置。
type RuntimeConfig struct {
	Site conf.Site `json:"site"`
	AI   conf.AI   `json:"ai"`
}

// RuntimeConfigService 负责运行时站点配置的加载、缓存与更新。
//
// 该服务通过构造器显式注入，避免 package 级全局状态。
type RuntimeConfigService struct {
	baseSite   conf.Site
	baseAI     conf.AI
	logger     *logrus.Logger
	db         *gorm.DB
	configFile string

	mu    sync.RWMutex
	cache *RuntimeConfig
}

// NewRuntimeConfigService 构造服务。
func NewRuntimeConfigService(baseSite conf.Site, baseAI conf.AI, logger *logrus.Logger, db *gorm.DB, configFile string) *RuntimeConfigService {
	return &RuntimeConfigService{
		baseSite:   baseSite,
		baseAI:     baseAI,
		logger:     logger,
		db:         db,
		configFile: configFile,
	}
}

func (s *RuntimeConfigService) runtimeBaseConfig() RuntimeConfig {
	return RuntimeConfig{
		Site: s.baseSite,
		AI:   s.baseAI,
	}
}

func (s *RuntimeConfigService) defaultRuntimeConfig() *RuntimeConfig {
	baseConfig := s.runtimeBaseConfig()
	return &RuntimeConfig{
		Site: baseConfig.Site,
		AI:   baseConfig.AI,
	}
}

func (s *RuntimeConfigService) loadRuntimeDefaultConfigFromFile() (*RuntimeConfig, error) {
	if s.configFile == "" {
		return nil, apperr.New(apperr.CodeRuntimeConfigPathMissing)
	}
	defaultFile := filepath.Join(filepath.Dir(s.configFile), "site_default_settings.yaml")
	byteData, err := os.ReadFile(defaultFile)
	if err != nil {
		return nil, err
	}

	var raw conf.RuntimeSiteDefault
	if err = envyaml.Unmarshal(byteData, &raw); err != nil {
		return nil, apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
	}
	return &RuntimeConfig{
		Site: raw.Site,
		AI:   raw.AI,
	}, nil
}

func (s *RuntimeConfigService) initialRuntimeConfig() *RuntimeConfig {
	cfg, err := s.loadRuntimeDefaultConfigFromFile()
	if err == nil {
		return cfg
	}
	if s.logger != nil {
		s.logger.Warnf("读取 site_default_settings.yaml 失败，回退到进程内默认值: %v", err)
	}
	return s.defaultRuntimeConfig()
}

func cloneRuntimeConfig(cfg *RuntimeConfig) *RuntimeConfig {
	if cfg == nil {
		return &RuntimeConfig{}
	}
	cp := *cfg
	return &cp
}

func (s *RuntimeConfigService) setRuntimeConfigCache(cfg *RuntimeConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = cloneRuntimeConfig(cfg)
}

// GetRuntimeConfig 返回当前生效的完整配置。
func (s *RuntimeConfigService) GetRuntimeConfig() RuntimeConfig {
	s.mu.RLock()
	if s.cache != nil {
		cfg := *s.cache
		s.mu.RUnlock()
		return cfg
	}
	s.mu.RUnlock()

	cfg := s.defaultRuntimeConfig()
	return *cfg
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

func (s *RuntimeConfigService) applyRuntimeConfig(cfg *RuntimeConfig) {
	s.setRuntimeConfigCache(cfg)
}

func marshalRuntimeConfig(cfg *RuntimeConfig) (siteJSON string, aiJSON string, err error) {
	siteData, err := json.Marshal(cfg.Site)
	if err != nil {
		return "", "", apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
	}
	aiData, err := json.Marshal(cfg.AI)
	if err != nil {
		return "", "", apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
	}
	return string(siteData), string(aiData), nil
}

func decodeRuntimeConfig(model *runtimeSiteConfigRow) (*RuntimeConfig, error) {
	cfg := &RuntimeConfig{}
	if model == nil {
		return cfg, nil
	}
	if model.SiteJSON != "" {
		if err := json.Unmarshal([]byte(model.SiteJSON), &cfg.Site); err != nil {
			return nil, apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
		}
	}
	if model.AIJSON != "" {
		if err := json.Unmarshal([]byte(model.AIJSON), &cfg.AI); err != nil {
			return nil, apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
		}
	}
	return cfg, nil
}

func (s *RuntimeConfigService) ensureRuntimeConfigModel(tx *gorm.DB) (*runtimeSiteConfigRow, *RuntimeConfig, error) {
	var model runtimeSiteConfigRow
	err := tx.Where("name = ?", runtimeConfigName).Take(&model).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		cfg := s.initialRuntimeConfig()
		siteJSON, aiJSON, marshalErr := marshalRuntimeConfig(cfg)
		if marshalErr != nil {
			return nil, nil, marshalErr
		}
		model = runtimeSiteConfigRow{
			Name:     runtimeConfigName,
			SiteJSON: siteJSON,
			AIJSON:   aiJSON,
		}
		if createErr := tx.Create(&model).Error; createErr != nil {
			return nil, nil, createErr
		}
		return &model, cfg, nil
	case err != nil:
		return nil, nil, err
	default:
		cfg, decodeErr := decodeRuntimeConfig(&model)
		if decodeErr != nil {
			return nil, nil, decodeErr
		}
		if cfg.Site.SiteInfo.Title == "" {
			cfg.Site = s.defaultRuntimeConfig().Site
		}
		if cfg.AI.ChatModel == "" && cfg.AI.BaseURL == "" && cfg.AI.SecretKey == "" {
			cfg.AI = s.defaultRuntimeConfig().AI
		}
		return &model, cfg, nil
	}
}

// InitRuntimeConfig 会在启动时把运行时站点配置从数据库加载到内存。
// 当数据库还没有初始化记录时，会使用 site_default_settings.yaml 中的默认值灌入数据库。
func (s *RuntimeConfigService) InitRuntimeConfig() error {
	if s.db == nil {
		return apperr.New(apperr.CodeRuntimeConfigDBMissing)
	}
	if !s.db.Migrator().HasTable(&runtimeSiteConfigRow{}) {
		return apperr.New(apperr.CodeRuntimeConfigTableMissing)
	}
	var loaded *RuntimeConfig
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		_, cfg, err := s.ensureRuntimeConfigModel(tx)
		if err != nil {
			return err
		}
		loaded = cfg
		return nil
	}); err != nil {
		return err
	}
	s.applyRuntimeConfig(loaded)
	return nil
}

// UpdateRuntimeSite 覆盖站点配置并刷新缓存。
func (s *RuntimeConfigService) UpdateRuntimeSite(site conf.Site) error {
	return s.update(func(cfg *RuntimeConfig) { cfg.Site = site })
}

// UpdateRuntimeAI 覆盖 AI 配置并刷新缓存。
func (s *RuntimeConfigService) UpdateRuntimeAI(ai conf.AI) error {
	return s.update(func(cfg *RuntimeConfig) { cfg.AI = ai })
}

// update 是两条更新路径的公共骨架：事务里改 JSON、提交后刷新内存缓存。
func (s *RuntimeConfigService) update(mutate func(*RuntimeConfig)) error {
	if s.db == nil {
		return apperr.New(apperr.CodeRuntimeConfigDBMissing)
	}
	var updated *RuntimeConfig
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		model, cfg, err := s.ensureRuntimeConfigModel(tx)
		if err != nil {
			return err
		}
		mutate(cfg)
		siteJSON, aiJSON, marshalErr := marshalRuntimeConfig(cfg)
		if marshalErr != nil {
			return marshalErr
		}
		if err = tx.Model(model).Updates(map[string]any{
			"site_json": siteJSON,
			"ai_json":   aiJSON,
		}).Error; err != nil {
			return err
		}
		updated = cfg
		return nil
	}); err != nil {
		return err
	}
	s.applyRuntimeConfig(updated)
	return nil
}
