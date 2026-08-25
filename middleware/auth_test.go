package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/performance_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestIsUserExpired(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	tests := []struct {
		name      string
		user      *model.UserBase
		wantValue bool
	}{
		{name: "nil user", user: nil, wantValue: false},
		{name: "permanent", user: &model.UserBase{ExpiresAt: 0}, wantValue: false},
		{name: "future expiration", user: &model.UserBase{ExpiresAt: now.Unix() + 1}, wantValue: false},
		{name: "expires now", user: &model.UserBase{ExpiresAt: now.Unix()}, wantValue: true},
		{name: "past expiration", user: &model.UserBase{ExpiresAt: now.Unix() - 1}, wantValue: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantValue, isUserExpired(tt.user, now))
		})
	}
}

func TestAbortExpiredUserUsesInsufficientQuotaContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	abortExpiredUser(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	var response struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.NotEmpty(t, response.Error.Message)
	assert.Equal(t, "new_api_error", response.Error.Type)
	assert.Equal(t, string(types.ErrorCodeInsufficientUserQuota), response.Error.Code)
}

func TestUserExpirationAuthRejectsExpiredUserBeforeHandler(t *testing.T) {
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	originalDB := model.DB
	originalRedisEnabled := common.RedisEnabled
	model.DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = originalDB
		common.RedisEnabled = originalRedisEnabled
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	user := model.User{
		Username:  "expired-middleware-user",
		Password:  "password",
		Status:    common.UserStatusEnabled,
		ExpiresAt: time.Now().Add(-time.Second).Unix(),
	}
	require.NoError(t, db.Create(&user).Error)

	handlerCalled := false
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", user.Id)
	})
	router.Use(UserExpirationAuth())
	router.POST("/pg/chat/completions", func(c *gin.Context) {
		handlerCalled = true
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/pg/chat/completions", nil)
	router.ServeHTTP(recorder, request)

	assert.False(t, handlerCalled)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Contains(t, recorder.Body.String(), string(types.ErrorCodeInsufficientUserQuota))
}

func TestTokenAuthAppliesConcurrentIPPolicy(t *testing.T) {
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	originalDB := model.DB
	originalRedisEnabled := common.RedisEnabled
	originalMaxIPs := performance_setting.GetPerformanceSetting().MaxIPsPerUser
	originalIsMasterNode := common.IsMasterNode
	originalSQLitePath := common.SQLitePath
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	common.IsMasterNode = false
	common.SQLitePath = dsn
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Setenv("SQL_DSN", "local")
	require.NoError(t, model.InitDB())
	db := model.DB
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))

	common.RedisEnabled = false
	performance_setting.GetPerformanceSetting().MaxIPsPerUser = 1
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = originalDB
		common.RedisEnabled = originalRedisEnabled
		performance_setting.GetPerformanceSetting().MaxIPsPerUser = originalMaxIPs
		common.IsMasterNode = originalIsMasterNode
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
	})

	user := model.User{
		Username: "model-list-ip-limit-user",
		Password: "password",
		Status:   common.UserStatusEnabled,
		Quota:    1000,
		Group:    "default",
	}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{
		UserId:         user.Id,
		Key:            "modellistiplimitkey",
		Status:         common.TokenStatusEnabled,
		Name:           "model-list-ip-limit-token",
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}
	require.NoError(t, db.Create(&token).Error)

	gin.SetMode(gin.TestMode)
	limitedEntered := make(chan struct{})
	limitedUnblock := make(chan struct{})
	disabledEntered := make(chan struct{})
	disabledUnblock := make(chan struct{})
	var responseHandlerCalls atomic.Int32
	router := gin.New()
	router.Use(TokenAuth())
	router.POST("/v1/responses", func(c *gin.Context) {
		switch responseHandlerCalls.Add(1) {
		case 1:
			close(limitedEntered)
			<-limitedUnblock
		case 2:
			close(disabledEntered)
			<-disabledUnblock
		}
		c.Status(http.StatusOK)
	})
	router.GET("/v1/models", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		request.Header.Set("Authorization", "Bearer sk-"+token.Key)
		request.RemoteAddr = "203.0.113.1:12345"
		router.ServeHTTP(recorder, request)
		firstDone <- recorder
	}()
	select {
	case <-limitedEntered:
	case recorder := <-firstDone:
		require.Failf(t, "first request did not reach handler", "status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	modelsRecorder := httptest.NewRecorder()
	modelsRequest := httptest.NewRequest(http.MethodGet, "/v1/models?client_version=test", nil)
	modelsRequest.Header.Set("Authorization", "Bearer sk-"+token.Key)
	modelsRequest.RemoteAddr = "198.51.100.2:12345"
	router.ServeHTTP(modelsRecorder, modelsRequest)

	responsesRecorder := httptest.NewRecorder()
	responsesRequest := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	responsesRequest.Header.Set("Authorization", "Bearer sk-"+token.Key)
	responsesRequest.RemoteAddr = "198.51.100.2:12345"
	router.ServeHTTP(responsesRecorder, responsesRequest)

	close(limitedUnblock)
	assert.Equal(t, http.StatusOK, (<-firstDone).Code)
	assert.Equal(t, http.StatusOK, modelsRecorder.Code)
	assert.Equal(t, http.StatusTooManyRequests, responsesRecorder.Code)

	disabled := false
	userSetting := user.GetSetting()
	userSetting.IPLimitEnabled = &disabled
	require.NoError(t, model.UpdateUserSetting(user.Id, userSetting))

	disabledFirstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		request.Header.Set("Authorization", "Bearer sk-"+token.Key)
		request.RemoteAddr = "203.0.113.1:12345"
		router.ServeHTTP(recorder, request)
		disabledFirstDone <- recorder
	}()
	select {
	case <-disabledEntered:
	case recorder := <-disabledFirstDone:
		require.Failf(t, "disabled-limit request did not reach handler", "status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	disabledSecondRecorder := httptest.NewRecorder()
	disabledSecondRequest := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	disabledSecondRequest.Header.Set("Authorization", "Bearer sk-"+token.Key)
	disabledSecondRequest.RemoteAddr = "198.51.100.2:12345"
	router.ServeHTTP(disabledSecondRecorder, disabledSecondRequest)

	close(disabledUnblock)
	assert.Equal(t, http.StatusOK, (<-disabledFirstDone).Code)
	assert.Equal(t, http.StatusOK, disabledSecondRecorder.Code)
	assert.Equal(t, int32(3), responseHandlerCalls.Load())
}

func TestAPIKeyFromWebSocketSubprotocol(t *testing.T) {
	tests := []struct {
		name      string
		protocols string
		wantKey   string
		wantOK    bool
	}{
		{
			name:      "responses protocol only",
			protocols: "responses",
			wantOK:    false,
		},
		{
			name:      "realtime protocol only",
			protocols: "realtime",
			wantOK:    false,
		},
		{
			name:      "responses with insecure key",
			protocols: "responses, openai-insecure-api-key.sk-test",
			wantKey:   "sk-test",
			wantOK:    true,
		},
		{
			name:      "realtime with beta and insecure key",
			protocols: "realtime, openai-insecure-api-key.sk-realtime, openai-beta.realtime-v1",
			wantKey:   "sk-realtime",
			wantOK:    true,
		},
		{
			name:      "empty insecure key",
			protocols: "responses, openai-insecure-api-key.",
			wantOK:    false,
		},
		{
			name:      "bare insecure marker is not a key",
			protocols: "openai-insecure-api-key",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKey, gotOK := apiKeyFromWebSocketSubprotocol(tt.protocols)
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotKey != tt.wantKey {
				t.Fatalf("key = %q, want %q", gotKey, tt.wantKey)
			}
		})
	}
}

func TestApplyWebSocketSubprotocolAuthorizationDoesNotOverrideProtocolOnly(t *testing.T) {
	header := http.Header{}
	header.Set("Authorization", "Bearer sk-original")
	header.Set("Sec-WebSocket-Protocol", "responses")

	if applyWebSocketSubprotocolAuthorization(header) {
		t.Fatal("authorization was unexpectedly applied")
	}
	if got := header.Get("Authorization"); got != "Bearer sk-original" {
		t.Fatalf("Authorization = %q, want original bearer", got)
	}
}

func TestApplyWebSocketSubprotocolAuthorizationOverridesWithInsecureKey(t *testing.T) {
	header := http.Header{}
	header.Set("Authorization", "Bearer sk-original")
	header.Set("Sec-WebSocket-Protocol", "responses, openai-insecure-api-key.sk-from-protocol")

	if !applyWebSocketSubprotocolAuthorization(header) {
		t.Fatal("authorization was not applied")
	}
	if got := header.Get("Authorization"); got != "Bearer sk-from-protocol" {
		t.Fatalf("Authorization = %q, want protocol bearer", got)
	}
}
