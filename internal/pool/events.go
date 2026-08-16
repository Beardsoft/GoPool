package pool

import (
	"encoding/json"
	"sync"
	"time"
)

type PoolEvent struct {
	Type      string          `json:"type"`
	Timestamp int64           `json:"ts"`
	Data      json.RawMessage `json:"data,omitempty"`
}

type Broadcaster struct {
	mu   sync.RWMutex
	subs map[chan PoolEvent]struct{}
}

var (
	broadcasterOnce   sync.Once
	globalBroadcaster *Broadcaster
)

func GetBroadcaster() *Broadcaster {
	broadcasterOnce.Do(func() {
		globalBroadcaster = &Broadcaster{
			subs: make(map[chan PoolEvent]struct{}),
		}
	})
	return globalBroadcaster
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subs: make(map[chan PoolEvent]struct{}),
	}
}

func (b *Broadcaster) Subscribe() chan PoolEvent {
	ch := make(chan PoolEvent, 32)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Broadcaster) Unsubscribe(ch chan PoolEvent) {
	b.mu.Lock()
	delete(b.subs, ch)
	close(ch)
	b.mu.Unlock()
}

func (b *Broadcaster) Publish(e PoolEvent) {
	b.mu.RLock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
	b.mu.RUnlock()
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func PublishEvent(eventType string, data any) {
	b := GetBroadcaster()
	raw, _ := json.Marshal(data)
	e := PoolEvent{
		Type:      eventType,
		Timestamp: time.Now().UnixMilli(),
		Data:      raw,
	}
	b.Publish(e)
}
