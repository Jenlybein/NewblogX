package jwts

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"myblogx/conf"
	"myblogx/models/ctype"
	"myblogx/models/enum"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
)

var (
	Method = jwt.SigningMethodHS256 // 签名算法
)

type Claims struct {
	UserID       ctype.ID      `json:"user_id"`
	SessionID    ctype.ID      `json:"session_id"`
	TokenVersion uint32        `json:"token_version"`
	Role         enum.RoleType `json:"role"`
	Username     string        `json:"username"`
}

type MyClaims struct {
	Claims
	jwt.StandardClaims
}

// 生成 token 的工具函数
func GetToken(config conf.Jwt, claims Claims) (string, error) {
	// 从配置文件中获取 jwt 相关配置
	var (
		TokenExpireDuration = time.Duration(config.Expire) * time.Hour // 令牌过期时间
		Secret              = []byte(config.Secret)                    // 密钥
		Issuer              = config.Issuer                            // jwt 签发者
	)

	// 构造自定义的Claims（JWT的载荷部分）
	myclaims := MyClaims{
		Claims: claims,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(TokenExpireDuration).Unix(), // 过期时间
			Issuer:    Issuer,                                     // 签发人
		},
	}
	// 创建Token对象，指定签名算法和载荷
	token := jwt.NewWithClaims(Method, myclaims)

	// 用密钥对Token进行签名，生成最终的token字符串
	tokenString, err := token.SignedString(Secret)
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

// 解析 token 的工具函数
func ParseToken(config conf.Jwt, tokenString string) (*MyClaims, error) {
	if tokenString == "" {
		return nil, errors.New("请登录：token 为空")
	}

	// 从配置文件中获取 jwt 相关配置
	var (
		Secret = []byte(config.Secret) // 密钥
	)

	// 解析 tokenString 字符串到指定的 MyClaims 结构体
	token, err := jwt.ParseWithClaims(tokenString, &MyClaims{}, func(token *jwt.Token) (any, error) {
		// 密钥回调函数：返回签名时使用的密钥（必须和生成Token时的Secret一致）
		if token.Method != Method {
			// 验证签名算法是否对应得上
			return nil, fmt.Errorf("token 签名算法错误: %w", jwt.ErrSignatureInvalid)
		}
		return Secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("token 解析错误: %w", err)
	}

	// 验证Token的有效性，并类型断言提取自定义Claims
	if claims, ok := token.Claims.(*MyClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("token 无效")
}

func GetTokenByGin(c *gin.Context) string {
	authHeader := strings.TrimSpace(c.Request.Header.Get("Authorization"))
	if authHeader != "" {
		if len(authHeader) > 7 && strings.EqualFold(authHeader[:7], "Bearer ") {
			return strings.TrimSpace(authHeader[7:])
		}
		return authHeader
	}

	if tokenString := strings.TrimSpace(c.Request.Header.Get("token")); tokenString != "" {
		return tokenString
	}
	return strings.TrimSpace(c.Request.Header.Get("Token"))
}

func ParseTokenByGin(c *gin.Context) (*MyClaims, error) {
	tokenString := GetTokenByGin(c)
	if c == nil {
		return nil, errors.New("请登录：token 为空")
	}
	value, ok := c.Get("_jwt_config")
	if !ok {
		return nil, errors.New("jwt 配置缺失")
	}
	jwtConfig, ok := value.(conf.Jwt)
	if !ok {
		return nil, errors.New("jwt 配置类型错误")
	}
	return ParseToken(jwtConfig, tokenString)
}

func GetClaimsByGin(c *gin.Context) (claims *MyClaims) {
	if rawClaims, ok := c.Get("claims"); ok {
		if parsedClaims, ok := rawClaims.(*MyClaims); ok {
			return parsedClaims
		}
	}
	return nil
}

func MustGetClaimsByGin(c *gin.Context) (claims *MyClaims) {
	return c.MustGet("claims").(*MyClaims)
}

func (claims *MyClaims) IsAdmin() bool {
	return claims.Role == enum.RoleAdmin
}

func (claims *MyClaims) IsUser() bool {
	return claims.Role == enum.RoleUser
}
