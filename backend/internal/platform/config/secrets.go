package config

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"myblogx/conf"
	"myblogx/internal/apperr"
	"myblogx/internal/platform/secretbox"
)

// 本文件负责"运行时配置里的密钥不以明文落库"。
//
// 三条不变量：
//
//  1. **写库前加密**：mail.secret_key / storage.secret_key / oauth.app_key
//     在序列化成 JSON 之前先过 secretbox，库里只出现 `enc:v1:...`。
//  2. **内存里是明文**：缓存与各段的 Getter 拿到的仍是明文，调用方（发信、上传、
//     OAuth 跳转）无需知道加密存在。
//  3. **后台仍然可配**：接口回显的是占位符（`******`），管理员照旧能填新值覆盖；
//     只是永远读不回旧值。所以"能在后台配"与"库里不存明文"同时成立。
//
// 为什么不在 HTTP 层做：具名出入口、全量解码、补种默认值三条路径都会碰这些字段，
// 在用例层做一次，三条路径自动一致。

// SetSealer 注入主密钥。
//
// 允许为 nil：没配 `BLOGX_CONFIG_SECRET_KEY` 的部署仍要能启动（读历史明文、
// 发信、上传都正常），只是**不能再写入新的密钥** —— 那会在 Update* 时明确报错，
// 而不是悄悄存明文。
func (s *RuntimeConfigService) SetSealer(box *secretbox.Box) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sealer = box
}

// Sealer 返回当前主密钥（测试与迁移命令用）。
func (s *RuntimeConfigService) Sealer() *secretbox.Box {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sealer
}

// sealForStore 返回"要落库的那一份"：敏感字段换成密文，其余原样。
//
// 返回副本而不是就地改：调用方传进来的是要进缓存的明文，就地改会让缓存里也变成密文。
func (s *RuntimeConfigService) sealForStore(name SectionName, value any) (any, error) {
	box := s.Sealer()
	switch v := value.(type) {
	case conf.Mail:
		sealed, err := box.Seal(v.SecretKey)
		if err != nil {
			return nil, secretStoreError(err)
		}
		v.SecretKey = sealed
		return v, nil
	case conf.QiNiu:
		sealed, err := box.Seal(v.SecretKey)
		if err != nil {
			return nil, secretStoreError(err)
		}
		v.SecretKey = sealed
		return v, nil
	case conf.QQ:
		sealed, err := box.Seal(v.AppKey)
		if err != nil {
			return nil, secretStoreError(err)
		}
		v.AppKey = sealed
		return v, nil
	default:
		_ = name
		return value, nil
	}
}

// openForUse 把内存里的密文解回明文。
//
// 解密失败**不清空也不中断**：清空会让"主密钥配错了"表现成"邮件突然发不出去"，
// 而真正的原因（密钥不匹配）只会在日志里；这里保留密文，让下一次请求继续报同样的
// 错，同时打一条明确的日志。
func (s *RuntimeConfigService) openForUse(cfg *RuntimeConfig) {
	box := s.Sealer()
	if box == nil {
		return
	}

	if secretbox.IsSealed(cfg.Mail.SecretKey) {
		plain, err := box.Open(cfg.Mail.SecretKey)
		if err != nil {
			s.warnOpenFailed("mail.secret_key", err)
		} else {
			cfg.Mail.SecretKey = plain
		}
	}
	if secretbox.IsSealed(cfg.Storage.SecretKey) {
		plain, err := box.Open(cfg.Storage.SecretKey)
		if err != nil {
			s.warnOpenFailed("storage.secret_key", err)
		} else {
			cfg.Storage.SecretKey = plain
		}
	}
	if secretbox.IsSealed(cfg.OAuth.AppKey) {
		plain, err := box.Open(cfg.OAuth.AppKey)
		if err != nil {
			s.warnOpenFailed("oauth.app_key", err)
		} else {
			cfg.OAuth.AppKey = plain
		}
	}
}

// secretStoreError 把"没有主密钥"翻成面向运维的错误码。
//
// 其它错误（随机数、加密失败）原样透出：那些是真 bug，不该被一个"去配环境变量"
// 的提示盖住。
func secretStoreError(err error) error {
	if errors.Is(err, secretbox.ErrNoMasterKey) {
		return apperr.New(apperr.CodeRuntimeConfigSecretUnavailable)
	}
	return apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
}

// EncryptStoredSecrets 把库里已存的明文密钥加密，返回实际改动的段数。
//
// 幂等：已是密文的值跳过。只处理三个敏感字段，其余配置原样保留 ——
// 迁移命令的职责范围越小越好，否则一次误改会波及与安全无关的设置。
func (s *RuntimeConfigService) EncryptStoredSecrets(box *secretbox.Box) (int, error) {
	if s.db == nil {
		return 0, apperr.New(apperr.CodeRuntimeConfigDBMissing)
	}
	if box == nil {
		return 0, apperr.New(apperr.CodeRuntimeConfigSecretUnavailable)
	}

	changed := 0
	err := s.db.Transaction(func(tx *gorm.DB) error {
		payloads, err := loadPayloads(tx)
		if err != nil {
			return err
		}

		// 每一段：解码成强类型（缺字段用默认值补齐）→ 加密 → 写回。
		if _, ok := payloads[string(SectionMail)]; ok {
			mail, err := decodeSection(SectionMail, s.defaults.Mail, payloads)
			if err != nil {
				return err
			}
			if !secretbox.IsSealed(mail.SecretKey) && mail.SecretKey != "" {
				if mail.SecretKey, err = box.Seal(mail.SecretKey); err != nil {
					return secretStoreError(err)
				}
				if err := writeSection(tx, SectionMail, mail); err != nil {
					return err
				}
				changed++
			}
		}

		if _, ok := payloads[string(SectionStorage)]; ok {
			storage, err := decodeSection(SectionStorage, s.defaults.Storage, payloads)
			if err != nil {
				return err
			}
			if !secretbox.IsSealed(storage.SecretKey) && storage.SecretKey != "" {
				if storage.SecretKey, err = box.Seal(storage.SecretKey); err != nil {
					return secretStoreError(err)
				}
				if err := writeSection(tx, SectionStorage, storage); err != nil {
					return err
				}
				changed++
			}
		}

		if _, ok := payloads[string(SectionOAuth)]; ok {
			oauth, err := decodeSection(SectionOAuth, s.defaults.OAuth, payloads)
			if err != nil {
				return err
			}
			if !secretbox.IsSealed(oauth.AppKey) && oauth.AppKey != "" {
				if oauth.AppKey, err = box.Seal(oauth.AppKey); err != nil {
					return secretStoreError(err)
				}
				if err := writeSection(tx, SectionOAuth, oauth); err != nil {
					return err
				}
				changed++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return changed, nil
}

// writeSection 把一段配置序列化后 upsert 进库。
func writeSection(tx *gorm.DB, name SectionName, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return apperr.Wrap(apperr.CodeRuntimeConfigDecodeFailed, err)
	}
	return upsertPayload(tx, string(name), string(raw))
}

func (s *RuntimeConfigService) warnOpenFailed(field string, err error) {
	if s.logger != nil {
		s.logger.Errorf("运行时配置：%s 解密失败，该字段保持不可用（检查 %s 是否与加密时一致）: %v",
			field, secretbox.EnvKey, err)
	}
}
