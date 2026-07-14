package wsmanager

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	KindRealtime  = "realtime"
	KindResponses = "responses"

	defaultCloseReason = "channel disabled or deleted"
	redisChannel       = "new-api:wsmanager:channel-close"
)

type entry struct {
	id        uint64
	channelID int
	kind      string
	info      ConnectionInfo
	close     func(reason string)
}

type closeEvent struct {
	ChannelIDs []int  `json:"channel_ids"`
	Reason     string `json:"reason"`
	Origin     string `json:"origin"`
}

type ConnectionInfo struct {
	ConnectionID uint64 `json:"connection_id"`
	UserID       int    `json:"user_id"`
	Username     string `json:"username"`
	TokenName    string `json:"token_name"`
	ChannelID    int    `json:"channel_id"`
	Kind         string `json:"kind"`
	Model        string `json:"model"`
	Transport    string `json:"transport"`
	ConnectedAt  int64  `json:"connected_at"`
	NodeName     string `json:"node_name"`
}

type ConnectionStats struct {
	TotalConnections int              `json:"total_connections"`
	TotalUsers       int              `json:"total_users"`
	Connections      []ConnectionInfo `json:"connections"`
}

var (
	mu       sync.Mutex
	nextID   uint64
	registry = map[int]map[uint64]*entry{}

	originOnce sync.Once
	originID   string

	subscriberOnce sync.Once
)

func Register(channelID int, kind string, close func(reason string)) func() {
	return RegisterWithInfo(channelID, kind, ConnectionInfo{}, close)
}

func RegisterWithInfo(channelID int, kind string, info ConnectionInfo, close func(reason string)) func() {
	if channelID <= 0 || close == nil {
		return func() {}
	}
	info.ChannelID = channelID
	info.Kind = kind
	if info.ConnectedAt <= 0 {
		info.ConnectedAt = time.Now().Unix()
	}
	if info.NodeName == "" {
		info.NodeName = common.NodeName
		if info.NodeName == "" {
			info.NodeName = "node"
		}
	}
	var closeOnce sync.Once
	safeClose := func(reason string) {
		closeOnce.Do(func() {
			close(reason)
		})
	}
	id := atomic.AddUint64(&nextID, 1)
	info.ConnectionID = id
	e := &entry{
		id:        id,
		channelID: channelID,
		kind:      kind,
		info:      info,
		close:     safeClose,
	}

	mu.Lock()
	if registry[channelID] == nil {
		registry[channelID] = map[uint64]*entry{}
	}
	registry[channelID][id] = e
	mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			mu.Lock()
			defer mu.Unlock()
			entries := registry[channelID]
			if entries == nil {
				return
			}
			delete(entries, id)
			if len(entries) == 0 {
				delete(registry, channelID)
			}
		})
	}
}

func GetConnectionStats() ConnectionStats {
	mu.Lock()
	defer mu.Unlock()

	connections := make([]ConnectionInfo, 0)
	users := make(map[string]struct{})
	for _, entries := range registry {
		for _, e := range entries {
			if e == nil {
				continue
			}
			connections = append(connections, e.info)
			if e.info.UserID > 0 {
				users[fmt.Sprintf("id:%d", e.info.UserID)] = struct{}{}
			} else if e.info.Username != "" {
				users["name:"+e.info.Username] = struct{}{}
			}
		}
	}
	sort.Slice(connections, func(i, j int) bool {
		if connections[i].ConnectedAt != connections[j].ConnectedAt {
			return connections[i].ConnectedAt < connections[j].ConnectedAt
		}
		if connections[i].Username != connections[j].Username {
			return connections[i].Username < connections[j].Username
		}
		return connections[i].ChannelID < connections[j].ChannelID
	})
	return ConnectionStats{
		TotalConnections: len(connections),
		TotalUsers:       len(users),
		Connections:      connections,
	}
}

func CloseChannel(channelID int, reason string) int {
	return CloseChannels([]int{channelID}, reason)
}

func CloseChannels(channelIDs []int, reason string) int {
	reason = normalizeReason(reason)
	entries := takeEntries(channelIDs)
	for _, e := range entries {
		e.close(reason)
	}
	if len(entries) > 0 {
		common.SysLog(fmt.Sprintf("closed %d active websocket connection(s), channels=%v, kinds=%v, reason=%s", len(entries), entryChannelIDs(entries), entryKindCounts(entries), reason))
	}
	return len(entries)
}

func CloseChannelsAndBroadcast(channelIDs []int, reason string) int {
	count := CloseChannels(channelIDs, reason)
	if err := PublishCloseChannels(context.Background(), channelIDs, reason); err != nil {
		common.SysLog(fmt.Sprintf("failed to publish websocket close event: %v", err))
	}
	return count
}

func StartSubscriber(ctx context.Context) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	subscriberOnce.Do(func() {
		go subscribe(ctx)
	})
}

func PublishCloseChannels(ctx context.Context, channelIDs []int, reason string) error {
	if !common.RedisEnabled || common.RDB == nil {
		return nil
	}
	ids := uniqueChannelIDs(channelIDs)
	if len(ids) == 0 {
		return nil
	}
	payload, err := common.Marshal(closeEvent{
		ChannelIDs: ids,
		Reason:     normalizeReason(reason),
		Origin:     getOriginID(),
	})
	if err != nil {
		return err
	}
	return common.RDB.Publish(ctx, redisChannel, payload).Err()
}

func subscribe(ctx context.Context) {
	pubsub := common.RDB.Subscribe(ctx, redisChannel)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var event closeEvent
			if err := common.Unmarshal([]byte(msg.Payload), &event); err != nil {
				common.SysLog(fmt.Sprintf("failed to unmarshal websocket close event: %v", err))
				continue
			}
			if event.Origin == getOriginID() {
				continue
			}
			CloseChannels(event.ChannelIDs, event.Reason)
		}
	}
}

func takeEntries(channelIDs []int) []*entry {
	ids := uniqueChannelIDs(channelIDs)
	if len(ids) == 0 {
		return nil
	}

	mu.Lock()
	defer mu.Unlock()

	var entries []*entry
	for _, channelID := range ids {
		for _, e := range registry[channelID] {
			entries = append(entries, e)
		}
		delete(registry, channelID)
	}
	return entries
}

func entryChannelIDs(entries []*entry) []int {
	ids := make([]int, 0, len(entries))
	for _, e := range entries {
		if e != nil {
			ids = append(ids, e.channelID)
		}
	}
	return uniqueChannelIDs(ids)
}

func entryKindCounts(entries []*entry) map[string]int {
	counts := make(map[string]int)
	for _, e := range entries {
		if e == nil {
			continue
		}
		counts[e.kind]++
	}
	return counts
}

func uniqueChannelIDs(channelIDs []int) []int {
	seen := make(map[int]struct{}, len(channelIDs))
	ids := make([]int, 0, len(channelIDs))
	for _, id := range channelIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func normalizeReason(reason string) string {
	if reason == "" {
		return defaultCloseReason
	}
	return reason
}

func getOriginID() string {
	originOnce.Do(func() {
		name := common.NodeName
		if name == "" {
			name = "node"
		}
		originID = fmt.Sprintf("%s-%d-%d", name, os.Getpid(), time.Now().UnixNano())
	})
	return originID
}
