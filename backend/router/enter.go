// 路由模块入口

package router

import (
	"errors"
	"net"
	"net/http"

	"myblogx/api"
	"myblogx/middleware"

	"github.com/gin-gonic/gin"
)

// Run 组装路由并启动 HTTP 服务。
//
// 返回 *http.Server 供上层注册到 Lifecycle.OnStop，实现优雅停机：
// 收到退出信号后先 Shutdown（停止接收新请求并等待在途请求结束），
// 而不是过去直接 r.Run(...) 阻塞、进程被信号硬杀。
//
// 监听句柄在返回前同步创建，端口被占用等绑定错误会直接返回 error，
// 避免"后台 goroutine 启动失败、主流程却挂着等信号"的假活状态。
func Run(deps middleware.Deps, app api.Api) (*http.Server, error) {
	gin.SetMode(deps.System.GinMode)
	r := gin.New()
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/health"},
	}))
	r.Use(gin.Recovery())
	r.Use(middleware.CorsMiddleware())
	mwRuntime := middleware.NewRuntime(deps)

	r.Static("/uploads", "./uploads")
	HealthRouter(r)

	nr := r.Group("/api")
	nr.Use(mwRuntime.LogMiddleware)

	SiteRouter(nr, mwRuntime)
	LogRouter(nr, app, mwRuntime)
	ImageRouter(nr, mwRuntime)
	BannerRouter(nr, mwRuntime)
	CaptchaRouter(nr, app, mwRuntime)
	UserRouter(nr, app, mwRuntime)
	ArticleRouter(nr, mwRuntime)
	CommentRouter(nr, mwRuntime)
	ChatRouter(nr, mwRuntime)
	SitemsgRouter(nr, mwRuntime)
	GlobalNotifRouter(nr, mwRuntime)
	FollowRouter(nr, mwRuntime)
	SearchRouter(nr, mwRuntime)
	AIRouter(nr, mwRuntime)
	DataRouter(nr, app, mwRuntime)

	ln, err := net.Listen("tcp", deps.System.Addr())
	if err != nil {
		return nil, err
	}

	srv := &http.Server{
		Addr:    ln.Addr().String(),
		Handler: r,
	}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if deps.Logger != nil {
				deps.Logger.Errorf("HTTP 服务异常退出: %v", err)
			}
		}
	}()
	return srv, nil
}
