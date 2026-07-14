package groupconcurrency

import (
	"sort"
	"sync"
)

const (
	TransportHTTP      = "http"
	TransportWebSocket = "ws"
)

type key struct {
	userID int
	group  string
}

type counter struct {
	username  string
	http      int
	websocket int
}

type Usage struct {
	UserID          int
	Username        string
	Group           string
	HTTPActive      int
	WebSocketActive int
}

var (
	mu       sync.Mutex
	counters = make(map[key]*counter)
)

func Acquire(userID int, username, group, transport string, limit int) (func(), bool) {
	usageKey := key{userID: userID, group: group}

	mu.Lock()
	current := counters[usageKey]
	active := 0
	if current != nil {
		active = current.http + current.websocket
	}
	if limit > 0 && active >= limit {
		mu.Unlock()
		return func() {}, false
	}
	if current == nil {
		current = &counter{}
		counters[usageKey] = current
	}
	if username != "" {
		current.username = username
	}
	if transport == TransportWebSocket {
		current.websocket++
	} else {
		current.http++
	}
	mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			mu.Lock()
			defer mu.Unlock()

			current := counters[usageKey]
			if current == nil {
				return
			}
			if transport == TransportWebSocket {
				current.websocket--
			} else {
				current.http--
			}
			if current.http == 0 && current.websocket == 0 {
				delete(counters, usageKey)
			}
		})
	}, true
}

func Snapshot() []Usage {
	mu.Lock()
	defer mu.Unlock()

	usage := make([]Usage, 0, len(counters))
	for usageKey, current := range counters {
		usage = append(usage, Usage{
			UserID:          usageKey.userID,
			Username:        current.username,
			Group:           usageKey.group,
			HTTPActive:      current.http,
			WebSocketActive: current.websocket,
		})
	}
	sort.Slice(usage, func(i, j int) bool {
		if usage[i].UserID != usage[j].UserID {
			return usage[i].UserID < usage[j].UserID
		}
		return usage[i].Group < usage[j].Group
	})
	return usage
}
