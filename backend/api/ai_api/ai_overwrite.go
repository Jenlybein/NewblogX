package ai_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/service/ai_service/ai_overwrite"

	"github.com/gin-gonic/gin"
)

// AIOverwriteView 对选中文本执行改写，并通过 SSE 直接输出 AI token 流。
func (h AIApi) AIOverwriteView(c *gin.Context) {
	if h.App.RuntimeSite == nil {
		res.SSEFail("运行时配置服务未初始化", c)
		return
	}
	cr := middleware.GetBindJson[AIOverwriteRequest](c)
	prepareAISSE(c)

	contentChan, errChan, err := ai_overwrite.RewriteContentStream(h.App.RuntimeSite.GetRuntimeAI(), cr.toServiceRequest())
	if err != nil {
		res.SSEFail(err.Error(), c)
		return
	}

	for contentChan != nil || errChan != nil {
		select {
		case text, ok := <-contentChan:
			if !ok {
				contentChan = nil
				continue
			}
			if text == "" {
				continue
			}
			res.SSEOk(AIBaseResponse{Content: text}, c)
		case streamErr, ok := <-errChan:
			if !ok {
				errChan = nil
				continue
			}
			if streamErr != nil {
				res.SSEFail(streamErr.Error(), c)
				return
			}
		}
	}
}
