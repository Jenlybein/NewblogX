package jwts_test

import (
	"myblogx/conf"
	"myblogx/models/enum"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestJWTGetAndParse(t *testing.T) {
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		Jwt: conf.Jwt{
			Expire: 1,
			Secret: "jwt-secret",
			Issuer: "blogx",
		},
	})
	jwtConf := testutil.Config().Jwt

	token, err := jwts.GetToken(jwtConf, jwts.Claims{
		UserID:   100,
		Role:     enum.RoleAdmin,
		Username: "root",
	})
	if err != nil {
		t.Fatalf("GetToken 失败: %v", err)
	}

	claims, err := jwts.ParseToken(jwtConf, token)
	if err != nil {
		t.Fatalf("ParseToken 失败: %v", err)
	}
	if claims.UserID != 100 || claims.Username != "root" {
		t.Fatalf("claims 异常: %+v", claims)
	}
}

func TestParseTokenByGin(t *testing.T) {
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		Jwt: conf.Jwt{
			Expire: 1,
			Secret: "jwt-secret",
			Issuer: "blogx",
		},
	})
	jwtConf := testutil.Config().Jwt

	token, err := jwts.GetToken(jwtConf, jwts.Claims{
		UserID:   10,
		Role:     enum.RoleUser,
		Username: "u10",
	})
	if err != nil {
		t.Fatalf("GetToken 失败: %v", err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	c.Request = req
	c.Set("_jwt_config", jwtConf)

	claims, err := jwts.ParseTokenByGin(c)
	if err != nil {
		t.Fatalf("ParseTokenByGin 失败: %v", err)
	}
	if claims.Username != "u10" {
		t.Fatalf("username 异常: %s", claims.Username)
	}
}
