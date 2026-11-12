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
// 段的类型各不相同，没法用一张表表达"这个字段是密钥"，所以仍是 type switch；
// 但每个 case 只有一行，加一个段也就一行。
func (s *RuntimeConfigService) sealForStore(name SectionName, value any) (any, error) {
	box := s.Sealer()
	switch v := value.(type) {
	case conf.Mail:
		return sealIn(box, &v, &v.SecretKey)
	case conf.QiNiu:
		return sealIn(box, &v, &v.SecretKey)
	case conf.QQ:
		return sealIn(box, &v, &v.AppKey)
	case conf.SMS:
		return sealIn(box, &v, &v.AccessKeySecret)
	default:
		_ = name
		return value, nil
	}
}

// sealIn 把 value 里的密钥字段就地换成密文后返回副本。
//
// field 必须指向 value 自己的字段：按值传 value、再传 &value.SecretKey 的话，
// 改的是调用方那个变量，返回的仍是加密前的副本 —— 库里就会落下明文。
func sealIn[T any](box *secretbox.Box, value *T, field *string) (any, error) {
	sealed, err := box.Seal(*field)
	if err != nil {
		return nil, secretStoreError(err)
	}
	*field = sealed
	return *value, nil
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

	s.openIn(box, "mail.secret_key", &cfg.Mail.SecretKey)
	s.openIn(box, "storage.secret_key", &cfg.Storage.SecretKey)
	s.openIn(box, "oauth.app_key", &cfg.OAuth.AppKey)
	s.openIn(box, "sms.access_key_secret", &cfg.SMS.AccessKeySecret)
}

// openIn 把单个字段的密文解成明文；不是密文就跳过，解密失败只记日志并保留密文。
func (s *RuntimeConfigService) openIn(box *secretbox.Box, label string, field *string) {
	if !secretbox.IsSealed(*field) {
		return
	}
	plain, err := box.Open(*field)
	if err != nil {
		s.warnOpenFailed(label, err)
		return
	}
	*field = plain
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

		// 每个敏感字段一次「解码 → 加密 → 写回」。四段各抄一遍曾是四个
		// 二十行的复制块：加第六个敏感字段时漏抄一处，那一处就永远明文落库，
		// 且没有任何测试会红。现在只有一份实现，每个字段一行。
		type step func() (bool, error)
		steps := []step{
			sealStep(tx, s, box, SectionMail, s.defaults.Mail, payloads,
				func(v *conf.Mail) *string { return &v.SecretKey }),
			sealStep(tx, s, box, SectionStorage, s.defaults.Storage, payloads,
				func(v *conf.QiNiu) *string { return &v.SecretKey }),
			sealStep(tx, s, box, SectionOAuth, s.defaults.OAuth, payloads,
				func(v *conf.QQ) *string { return &v.AppKey }),
			sealStep(tx, s, box, SectionSMS, s.defaults.SMS, payloads,
				func(v *conf.SMS) *string { return &v.AccessKeySecret }),
		}
		for _, run := range steps {
			sealed, err := run()
			if err != nil {
				return err
			}
			if sealed {
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

// sealStep 把"补加密某一段"包成迁移命令要的一个 step。
func sealStep[T any](
	tx *gorm.DB,
	svc *RuntimeConfigService,
	box *secretbox.Box,
	name SectionName,
	defaults T,
	payloads map[string]string,
	field func(*T) *string,
) func() (bool, error) {
	return func() (bool, error) {
		return sealSectionSecret(tx, svc, box, name, defaults, payloads, field)
	}
}

// sealSectionSecret 把某一段里的密钥字段加密后写回库，返回是否真的改动了。
//
// 已是密文或空值时跳过（幂等）；段不在库里时也跳过——补种由 seedMissing 负责，
// 迁移命令不该顺手创建新段。
func sealSectionSecret[T any](
	tx *gorm.DB,
	svc *RuntimeConfigService,
	box *secretbox.Box,
	name SectionName,
	defaults T,
	payloads map[string]string,
	field func(*T) *string,
) (bool, error) {
	if _, ok := payloads[string(name)]; !ok {
		return false, nil
	}
	value, err := decodeSection(name, defaults, payloads)
	if err != nil {
		return false, err
	}
	secret := *field(&value)
	if secretbox.IsSealed(secret) || secret == "" {
		return false, nil
	}
	sealed, err := box.Seal(secret)
	if err != nil {
		return false, secretStoreError(err)
	}
	*field(&value) = sealed
	if err := writeSection(tx, name, value); err != nil {
		return false, err
	}
	return true, nil
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
