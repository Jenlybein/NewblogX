package site_service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"myblogx/conf"
	siteconf "myblogx/conf/site"
	"myblogx/models"
	"myblogx/utils/envyaml"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const runtimeConfigName = "default"

type RuntimeConfig struct {
	Site conf.Site `json:"site"`
	AI   conf.AI   `json:"ai"`
}

// RuntimeConfigService 负责运行时站点配置的加载、缓存与更新。
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
		return nil, errors.New("运行时站点默认配置路径未设置")
	}
	defaultFile := filepath.Join(filepath.Dir(s.configFile), "site_default_settings.yaml")
	byteData, err := os.ReadFile(defaultFile)
	if err != nil {
		return nil, err
	}

	var raw conf.RuntimeSiteDefault
	if err = envyaml.Unmarshal(byteData, &raw); err != nil {
		return nil, fmt.Errorf("解析运行时站点默认配置失败: %w", err)
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

func (s *RuntimeConfigService) GetRuntimeSite() conf.Site {
	return s.GetRuntimeConfig().Site
}

func (s *RuntimeConfigService) GetRuntimeLogin() siteconf.Login {
	return s.GetRuntimeSite().Login
}

func (s *RuntimeConfigService) GetRuntimeArticle() siteconf.Article {
	return s.GetRuntimeSite().Article
}

func (s *RuntimeConfigService) GetRuntimeComment() siteconf.Comment {
	return s.GetRuntimeSite().Comment
}

func (s *RuntimeConfigService) GetRuntimeAI() conf.AI {
	return s.GetRuntimeConfig().AI
}

func (s *RuntimeConfigService) applyRuntimeConfig(cfg *RuntimeConfig) {
	s.setRuntimeConfigCache(cfg)
}

func marshalRuntimeConfig(cfg *RuntimeConfig) (siteJSON string, aiJSON string, err error) {
	siteData, err := json.Marshal(cfg.Site)
	if err != nil {
		return "", "", fmt.Errorf("序列化站点配置失败: %w", err)
	}
	aiData, err := json.Marshal(cfg.AI)
	if err != nil {
		return "", "", fmt.Errorf("序列化 AI 配置失败: %w", err)
	}
	return string(siteData), string(aiData), nil
}

func decodeRuntimeConfig(model *models.RuntimeSiteConfigModel) (*RuntimeConfig, error) {
	cfg := &RuntimeConfig{}
	if model == nil {
		return cfg, nil
	}
	if model.SiteJSON != "" {
		if err := json.Unmarshal([]byte(model.SiteJSON), &cfg.Site); err != nil {
			return nil, fmt.Errorf("解析站点配置失败: %w", err)
		}
	}
	if model.AIJSON != "" {
		if err := json.Unmarshal([]byte(model.AIJSON), &cfg.AI); err != nil {
			return nil, fmt.Errorf("解析 AI 配置失败: %w", err)
		}
	}
	return cfg, nil
}

func (s *RuntimeConfigService) ensureRuntimeConfigModel(tx *gorm.DB) (*models.RuntimeSiteConfigModel, *RuntimeConfig, error) {
	var model models.RuntimeSiteConfigModel
	err := tx.Where("name = ?", runtimeConfigName).Take(&model).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		cfg := s.initialRuntimeConfig()
		siteJSON, aiJSON, marshalErr := marshalRuntimeConfig(cfg)
		if marshalErr != nil {
			return nil, nil, marshalErr
		}
		model = models.RuntimeSiteConfigModel{
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
		return errors.New("数据库未初始化")
	}
	if !s.db.Migrator().HasTable(&models.RuntimeSiteConfigModel{}) {
		return errors.New("运行时配置表不存在，请先执行数据库初始化命令：/app/server -db")
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

func (s *RuntimeConfigService) UpdateRuntimeSite(site conf.Site) error {
	if s.db == nil {
		return errors.New("数据库未初始化")
	}
	var updated *RuntimeConfig
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		model, cfg, err := s.ensureRuntimeConfigModel(tx)
		if err != nil {
			return err
		}
		cfg.Site = site
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

func (s *RuntimeConfigService) UpdateRuntimeAI(ai conf.AI) error {
	if s.db == nil {
		return errors.New("数据库未初始化")
	}
	var updated *RuntimeConfig
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		model, cfg, err := s.ensureRuntimeConfigModel(tx)
		if err != nil {
			return err
		}
		cfg.AI = ai
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
