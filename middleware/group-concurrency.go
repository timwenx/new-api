package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/groupconcurrency"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
)

func modelRequestGroup(c *gin.Context) string {
	group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}
	return group
}

func GroupConcurrencyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		group := modelRequestGroup(c)
		limit := setting.GetGroupConcurrencyLimit(group)
		transport := groupconcurrency.TransportHTTP
		if c.Request != nil && strings.EqualFold(c.Request.Header.Get("Upgrade"), "websocket") {
			transport = groupconcurrency.TransportWebSocket
		}

		release, allowed := groupconcurrency.Acquire(
			c.GetInt("id"),
			common.GetContextKeyString(c, constant.ContextKeyUserName),
			group,
			transport,
			limit,
		)
		if !allowed {
			abortWithOpenAiMessage(
				c,
				http.StatusTooManyRequests,
				fmt.Sprintf("您已达到 %s 分组并发限制：HTTP 与 WebSocket 合计最多 %d 个请求", group, limit),
			)
			return
		}
		defer release()

		c.Next()
	}
}
