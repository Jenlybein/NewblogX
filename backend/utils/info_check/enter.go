package info_check

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
)

// 敏感用户名检测
var sensitiveWords = map[string]bool{
	// 系统高危称谓（避免冒充管理员/官方）
	"admin":         true,
	"root":          true,
	"administrator": true,
	"superuser":     true,
	"system":        true,
	"operator":      true,
	"moderator":     true,
	"gm":            true, // 游戏管理员
	"official":      true, // 官方
	"service":       true, // 客服
	"manager":       true, // 管理员

	// 辱骂/低俗类（英文核心违禁词）
	"fuck":     true,
	"suck":     true,
	"shit":     true,
	"bitch":    true,
	"asshole":  true,
	"dick":     true,
	"pussy":    true,
	"cunt":     true,
	"bastard":  true,
	"damn":     true,
	"shithead": true,
	"moron":    true, // 蠢货
	"idiot":    true, // 白痴
	"retard":   true, // 智障
	"loser":    true, // 废物
	"jerk":     true, // 混蛋
	"ass":      true, // 粗俗用语
	"piss":     true, // 粗俗用语
	"crap":     true, // 垃圾
	"wanker":   true, // 低俗用语
	"dumbass":  true, // 蠢蛋
	"douche":   true, // 低俗用语

	// 违规/违法相关（英文简版）
	"gambling": true, // 赌博
	"bet":      true, // 投注
	"porn":     true, // 色情
	"drug":     true, // 毒品
	"hack":     true, // 黑客
	"scam":     true, // 诈骗
	"fraud":    true, // 欺诈
	"gun":      true, // 枪支
	"kill":     true, // 杀人
	"suicide":  true, // 自杀
	"bomb":     true, // 爆炸
	"violence": true, // 暴力

	// 规避变种绕过（常见替换/变形）
	"fck":   true, // fuck变种
	"sh1t":  true, // shit变种（数字1替换i）
	"b1tch": true, // bitch变种
	"fuq":   true, // fuck变种
}

// 检查用户名是否包含敏感词
func IsSensitiveWord(word string) (sensitive string, ok bool) {
	lower := strings.ToLower(word)

	// 过滤：仅保留字母（剔除数字、下划线，聚焦核心违禁词）
	var cleanLetters strings.Builder
	for _, r := range lower {
		// 只保留a-z的字母，剔除数字、下划线
		if unicode.IsLetter(r) && unicode.IsLower(r) {
			cleanLetters.WriteRune(r)
		}
	}
	processedWord := cleanLetters.String()

	// 匹配违禁词（只要包含任意违禁词即返回true）
	for word := range sensitiveWords {
		if strings.Contains(processedWord, word) {
			return word, true
		}
	}

	return "", false
}

// 用户名正则表达式：仅允许字母、数字、下划线
var validUsernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func CheckUsername(username string) error {
	// 清除用户名首尾空格
	cleanUsername := strings.TrimSpace(username)
	if cleanUsername == "" {
		return errors.New("用户名不能为空")
	}

	usernameLen := len(cleanUsername)
	if usernameLen < 6 || usernameLen > 20 {
		return errors.New("用户名长度必须在6-20个字符之间")
	}

	if !validUsernameRegex.MatchString(cleanUsername) {
		return errors.New("用户名仅支持字母、数字、下划线")
	}

	underlineCount := strings.Count(cleanUsername, "_")
	if underlineCount > 2 {
		return errors.New("用户名中下划线数量最多只能有2个")
	}

	if strings.HasPrefix(cleanUsername, "_") || strings.HasSuffix(cleanUsername, "_") {
		return errors.New("用户名不能以下划线开头或结尾")
	}

	// 敏感词校验
	sensitive, ok := IsSensitiveWord(cleanUsername)
	if ok {
		return errors.New("用户名包含敏感词汇：" + sensitive)
	}

	return nil
}
