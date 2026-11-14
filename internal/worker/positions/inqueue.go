package positions

import "sync"

type inQueue struct {
	mu       sync.Mutex
	next     uint64
	versions map[string]uint64
}

func newInQueue() *inQueue {
	return &inQueue{versions: make(map[string]uint64)}
}

func (q *inQueue) add(tripID string) (uint64, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.versions[tripID]; exists {
		return 0, false
	}
	q.next++
	q.versions[tripID] = q.next
	return q.next, true
}

func (q *inQueue) cancel(tripID string) (uint64, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	version, exists := q.versions[tripID]
	delete(q.versions, tripID)
	return version, exists
}

func (q *inQueue) current(tripID string, version uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.versions[tripID] == version
}

func (q *inQueue) finish(tripID string, version uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.versions[tripID] != version {
		return false
	}
	delete(q.versions, tripID)
	return true
}
