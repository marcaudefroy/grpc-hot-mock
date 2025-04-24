package history

import (
	"sync"
	"time"
)

type History struct {
	Date      time.Time `json:"date"`
	Request   Request   `json:"request"`
	Response  Response  `json:"response"`
	Proxified bool      `json:"proxified"`
}

type Request struct {
	FullName      string `json:"service"`
	Recognized    bool   `json:"recognized"`
	PayloadString string `json:"payload_string"`
	Payload       any    `json:"payload"`
}

type Response struct {
	Status        int    `json:"status"`
	Payload       any    `json:"payload"`
	PayloadString string `json:"payload_string"`
}

type RegistryWriter interface {
	RegisterHistory(History)
}
type RegistryReader interface {
	GetHistories() []History
}

type DefaultRegistry struct {
	histories []History
	mu        sync.Mutex
}

func (r *DefaultRegistry) RegisterHistory(h History) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.histories = append(r.histories, h)
}

func (r *DefaultRegistry) GetHistories() []History {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.histories
}
