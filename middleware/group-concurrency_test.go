package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupConcurrencyLimitReleasesSlotWhenRequestCompletes(t *testing.T) {
	oldLimit := setting.GetModelRequestConcurrencyLimit()
	setting.SetModelRequestConcurrencyLimit(1)
	defer func() {
		setting.SetModelRequestConcurrencyLimit(oldLimit)
	}()

	gin.SetMode(gin.TestMode)
	entered := make(chan struct{})
	unblock := make(chan struct{})
	var handlerCalls atomic.Int32
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 987654)
		common.SetContextKey(c, constant.ContextKeyUserName, "concurrency-test-user")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "concurrency-test-group")
		c.Next()
	})
	router.Use(GroupConcurrencyLimit())
	router.GET("/v1/test", func(c *gin.Context) {
		if handlerCalls.Add(1) == 1 {
			entered <- struct{}{}
			<-unblock
		}
		c.Status(http.StatusOK)
	})

	server := httptest.NewServer(router)
	defer server.Close()

	firstDone := make(chan int, 1)
	go func() {
		response, err := http.Get(server.URL + "/v1/test")
		if err != nil {
			firstDone <- 0
			return
		}
		defer response.Body.Close()
		firstDone <- response.StatusCode
	}()
	<-entered

	response, err := http.Get(server.URL + "/v1/test")
	require.NoError(t, err)
	response.Body.Close()
	assert.Equal(t, http.StatusTooManyRequests, response.StatusCode)

	close(unblock)
	assert.Equal(t, http.StatusOK, <-firstDone)

	response, err = http.Get(server.URL + "/v1/test")
	require.NoError(t, err)
	response.Body.Close()
	assert.Equal(t, http.StatusOK, response.StatusCode)
}
