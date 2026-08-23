package useriplimit

import "sync"

var (
	mu             sync.Mutex
	activeRequests = make(map[int]map[string]int)
)

func Acquire(userID int, clientIP string, limit int) (func(), bool) {
	if userID <= 0 || clientIP == "" {
		return func() {}, true
	}

	mu.Lock()
	userRequests := activeRequests[userID]
	if userRequests == nil {
		userRequests = make(map[string]int)
		activeRequests[userID] = userRequests
	}
	if userRequests[clientIP] == 0 && limit > 0 && len(userRequests) >= limit {
		mu.Unlock()
		return func() {}, false
	}
	userRequests[clientIP]++
	mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			mu.Lock()
			defer mu.Unlock()

			userRequests := activeRequests[userID]
			if userRequests == nil {
				return
			}
			userRequests[clientIP]--
			if userRequests[clientIP] <= 0 {
				delete(userRequests, clientIP)
			}
			if len(userRequests) == 0 {
				delete(activeRequests, userID)
			}
		})
	}, true
}
