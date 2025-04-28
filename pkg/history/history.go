package history

import (
	"sort"
	"sync"
	"time"
)

type State string

const (
	StateOpen   State = "OPEN"
	StateClosed State = "CLOSED"
)

type History struct {
	ID          string     `json:"id"`
	StartTime   time.Time  `json:"start_time"`
	EndTime     *time.Time `json:"end_time"`
	FullMethod  string     `json:"full_method"`
	Messages    []Message  `json:"messages"`
	State       State      `json:"state"`
	GrpcCode    int32      `json:"grpc_code"`
	GrpcMessage string     `json:"grpc_message"`
}

type Message struct {
	Direction     string      `json:"direction"` // "recv" or "send"
	Timestamp     time.Time   `json:"timestamp"`
	Recognized    bool        `json:"recognized"`
	Proxified     bool        `json:"proxified"`
	PayloadString string      `json:"payload_string"`
	Payload       interface{} `json:"payload"`
}

type RegisterReadWriter interface {
	RegistryWriter
	RegistryReader
}

type RegistryWriter interface {
	SaveHistory(History)
	Clean()
}
type RegistryReader interface {
	GetHistories() []History
}

type DefaultRegistry struct {
	histories []History
	mu        sync.RWMutex
}

func (r *DefaultRegistry) SaveHistory(h History) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.histories {
		if r.histories[i].ID == h.ID {
			r.histories[i] = h
			return
		}
	}
	r.histories = append(r.histories, h)
}

func (r *DefaultRegistry) GetHistories() []History {
	r.mu.RLock()
	defer r.mu.RUnlock()

	histories := make([]History, len(r.histories))
	copy(histories, r.histories)

	sort.Slice(histories, func(i, j int) bool {
		return histories[i].StartTime.Before(histories[j].StartTime)
	})

	return histories
}

func (r *DefaultRegistry) Clean() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.histories = []History{}
}
