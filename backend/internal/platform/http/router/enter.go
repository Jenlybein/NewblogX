// 路由模块入口

package router

import (
	"errors"
	"myblogx/internal/platform/safego"
	"net"
	"net/http"

	"myblogx/internal/platform/http/middleware"

	"github.com/gin-gonic/gin"
)

// NewEngine 组装完整路由树，但不监听端口。
//
// 为什么要单独抽出来：`contract_routes_test.go` 要拿真实路由表和契约对账。
// 如果测试里另抄一份挂载顺序，将来在这里加了个 Router 而测试没跟上，
// 测试就会给出假绿（少挂的那部分没人对账）或假红（对着旧清单比）。
// 让生产与测试走同一个函数，是这类"清单必须与代码同步"的唯一可靠做法。
func NewEngine(opts middleware.RuntimeOptions) *gin.Engine {
	r := gin.New()
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/health"},
	}))
	r.Use(gin.Recovery())
	r.Use(middleware.CorsMiddleware())
	mwRuntime := middleware.NewRuntime(opts)

	r.Static("/uploads", "./uploads")
	HealthRouter(r)

	nr := r.Group("/api")
	nr.Use(mwRuntime.LogMiddleware)

	SiteRouter(nr, mwRuntime)
	LogRouter(nr, mwRuntime)
	ImageRouter(nr, mwRuntime)
	BannerRouter(nr, mwRuntime)
	CaptchaRouter(nr)
	UserRouter(nr, mwRuntime)
	ArticleRouter(nr, mwRuntime)
	CommentRouter(nr, mwRuntime)
	ChatRouter(nr, mwRuntime)
	SitemsgRouter(nr, mwRuntime)
	GlobalNotifRouter(nr, mwRuntime)
	FollowRouter(nr, mwRuntime)
	SearchRouter(nr, mwRuntime)
	AIRouter(nr, mwRuntime)
	DataRouter(nr, mwRuntime)

	return r
}

// Run 组装路由并启动 HTTP 服务。
//
// 返回 *http.Server 供上层注册到 Lifecycle.OnStop，实现优雅停机：
// 收到退出信号后先 Shutdown（停止接收新请求并等待在途请求结束），
// 而不是过去直接 r.Run(...) 阻塞、进程被信号硬杀。
//
// 监听句柄在返回前同步创建，端口被占用等绑定错误会直接返回 error，
// 避免"后台 goroutine 启动失败、主流程却挂着等信号"的假活状态。
func Run(opts middleware.RuntimeOptions) (*http.Server, error) {
	gin.SetMode(opts.System.GinMode)
	r := NewEngine(opts)

	ln, err := net.Listen("tcp", opts.System.Addr())
	if err != nil {
		return nil, err
	}

	srv := &http.Server{
		Addr:    ln.Addr().String(),
		Handler: r,
	}
	safego.Go(func(recovered any, stack []byte) {
		if opts.Logger != nil {
			opts.Logger.Errorf("HTTP 服务 panic: %v\n%s", recovered, stack)
		}
	}, func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if opts.Logger != nil {
				opts.Logger.Errorf("HTTP 服务异常退出: %v", err)
			}
		}
	})
	return srv, nil
}
