package common

import (
	"math"
	"sync/atomic"
	"time"
)

// PerformanceMonitorConfig 性能监控配置
type PerformanceMonitorConfig struct {
	Enabled                     bool
	CPUThreshold                int
	MemoryThreshold             int
	DiskThreshold               int
	WebSocketIdleTimeoutMinutes int
}

var performanceMonitorConfig atomic.Value

func init() {
	// 初始化默认配置
	performanceMonitorConfig.Store(PerformanceMonitorConfig{
		Enabled:                     true,
		CPUThreshold:                90,
		MemoryThreshold:             90,
		DiskThreshold:               90,
		WebSocketIdleTimeoutMinutes: 10,
	})
}

// GetPerformanceMonitorConfig 获取性能监控配置
func GetPerformanceMonitorConfig() PerformanceMonitorConfig {
	return performanceMonitorConfig.Load().(PerformanceMonitorConfig)
}

// SetPerformanceMonitorConfig 设置性能监控配置
func SetPerformanceMonitorConfig(config PerformanceMonitorConfig) {
	performanceMonitorConfig.Store(config)
}

// GetWebSocketIdleTimeout returns the client WebSocket application-message
// idle timeout. Zero disables idle disconnects.
func GetWebSocketIdleTimeout() time.Duration {
	minutes := GetPerformanceMonitorConfig().WebSocketIdleTimeoutMinutes
	if minutes <= 0 {
		return 0
	}
	maxMinutes := int64(math.MaxInt64) / int64(time.Minute)
	if int64(minutes) > maxMinutes {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(minutes) * time.Minute
}
