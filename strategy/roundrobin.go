package strategy

import (
	"github.com/hashicorp/go-hclog"
	"sync"
	"sync/atomic"
)

type Strategy interface {
	Next() string
	Set(targets []string)
}

type roundRobin struct {
	sync.RWMutex
	log     hclog.Logger
	targets []string
	next    uint64
}

func NewRoundRobin(log hclog.Logger, targets []string) *roundRobin {
	r := &roundRobin{
		log: log,
	}
	r.Set(targets)
	return r
}

func (r *roundRobin) Set(targets []string) {
	r.Lock()
	defer r.Unlock()

	r.targets = targets
}

func (r *roundRobin) Next() string {
	r.RLock()
	defer r.RUnlock()
	if len(r.targets) == 0 {
		return ""
	}
	n := atomic.AddUint64(&r.next, 1)
	return r.targets[n%uint64(len(r.targets))]
}
