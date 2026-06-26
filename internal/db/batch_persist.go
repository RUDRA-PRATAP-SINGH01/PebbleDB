package db

import "sync"

// batchPersistBarrier tracks in-flight WAL batch writes so Sync() can wait without
// racing sync.WaitGroup Add/Wait across goroutines (required for -race on Linux CI).
type batchPersistBarrier struct {
	mu       sync.Mutex
	inflight int
	cond     sync.Cond
}

func newBatchPersistBarrier() *batchPersistBarrier {
	b := &batchPersistBarrier{}
	b.cond.L = &b.mu
	return b
}

func (b *batchPersistBarrier) begin() {
	b.mu.Lock()
	b.inflight++
	b.mu.Unlock()
}

func (b *batchPersistBarrier) end() {
	b.mu.Lock()
	b.inflight--
	if b.inflight == 0 {
		b.cond.Broadcast()
	}
	b.mu.Unlock()
}

func (b *batchPersistBarrier) wait() {
	b.mu.Lock()
	for b.inflight > 0 {
		b.cond.Wait()
	}
	b.mu.Unlock()
}
