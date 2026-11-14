package positions

import (
	"sort"
	"sync"
)

type priorityQueue struct {
	mu       sync.Mutex
	notEmpty *sync.Cond
	items    []task
	capacity int
	closed   bool
}

func newPriorityQueue(capacity int) *priorityQueue {
	q := &priorityQueue{items: make([]task, 0, capacity), capacity: capacity}
	q.notEmpty = sync.NewCond(&q.mu)
	return q
}

func (t task) silentLongerThan(other task) bool {
	if !t.trip.LastActivityAt.Equal(other.trip.LastActivityAt) {
		return t.trip.LastActivityAt.Before(other.trip.LastActivityAt)
	}
	return t.version < other.version
}

func (q *priorityQueue) push(t task) (accepted bool, evicted *task) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == q.capacity {
		freshest := q.items[len(q.items)-1]
		if !t.silentLongerThan(freshest) {
			return false, nil
		}
		q.items = q.items[:len(q.items)-1]
		evicted = &freshest
	}

	at := sort.Search(len(q.items), func(i int) bool { return t.silentLongerThan(q.items[i]) })
	q.items = append(q.items, task{})
	copy(q.items[at+1:], q.items[at:])
	q.items[at] = t

	q.notEmpty.Signal()
	return true, evicted
}

func (q *priorityQueue) pop() (task, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.items) == 0 && !q.closed {
		q.notEmpty.Wait()
	}
	if len(q.items) == 0 {
		return task{}, false
	}

	t := q.items[0]
	copy(q.items, q.items[1:])
	q.items = q.items[:len(q.items)-1]
	return t, true
}

func (q *priorityQueue) remove(tripID string, version uint64) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, t := range q.items {
		if t.trip.TripID == tripID && t.version == version {
			copy(q.items[i:], q.items[i+1:])
			q.items = q.items[:len(q.items)-1]
			return
		}
	}
}

func (q *priorityQueue) size() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.items)
}

func (q *priorityQueue) shut() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	q.notEmpty.Broadcast()
}
