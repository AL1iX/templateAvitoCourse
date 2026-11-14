package positions

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/AL1iX/templateAvitoCourse/internal/model"
)

var queueBase = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func queued(id string, minute int, version uint64) task {
	return task{
		trip:    model.StaleTrip{TripID: id, LastActivityAt: queueBase.Add(time.Duration(minute) * time.Minute)},
		version: version,
	}
}

func popIDs(t *testing.T, q *priorityQueue, n int) []string {
	t.Helper()

	ids := make([]string, 0, n)
	for range n {
		got, ok := q.pop()
		require.True(t, ok)
		ids = append(ids, got.trip.TripID)
	}
	return ids
}

func TestPriorityQueuePopsTheLongestSilentFirst(t *testing.T) {
	q := newPriorityQueue(10)

	for _, tk := range []task{queued("c", 30, 1), queued("a", 10, 2), queued("d", 40, 3), queued("b", 20, 4)} {
		accepted, evicted := q.push(tk)
		require.True(t, accepted)
		require.Nil(t, evicted)
	}

	require.Equal(t, []string{"a", "b", "c", "d"}, popIDs(t, q, 4))
}

func TestPriorityQueueKeepsArrivalOrderForEqualActivity(t *testing.T) {
	q := newPriorityQueue(10)

	for i, id := range []string{"x", "y", "z"} {
		q.push(queued(id, 5, uint64(i+1)))
	}

	require.Equal(t, []string{"x", "y", "z"}, popIDs(t, q, 3))
}

func TestPriorityQueueEvictsTheFreshestForAnOlderTask(t *testing.T) {
	q := newPriorityQueue(2)
	q.push(queued("b", 20, 1))
	q.push(queued("c", 30, 2))

	accepted, evicted := q.push(queued("a", 10, 3))

	require.True(t, accepted)
	require.NotNil(t, evicted)
	require.Equal(t, "c", evicted.trip.TripID)
	require.Equal(t, 2, q.size())
	require.Equal(t, []string{"a", "b"}, popIDs(t, q, 2))
}

func TestPriorityQueueRejectsATaskThatIsNotSilentLongerThanTheFreshest(t *testing.T) {
	tests := []struct {
		name string
		task task
	}{
		{name: "fresher", task: queued("n", 40, 3)},
		{name: "equal activity, later arrival", task: queued("n", 30, 3)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newPriorityQueue(2)
			q.push(queued("b", 20, 1))
			q.push(queued("c", 30, 2))

			accepted, evicted := q.push(tt.task)

			require.False(t, accepted)
			require.Nil(t, evicted)
			require.Equal(t, []string{"b", "c"}, popIDs(t, q, 2))
		})
	}
}

func TestPriorityQueuePopWaitsForATask(t *testing.T) {
	q := newPriorityQueue(1)
	got := make(chan task, 1)
	go func() {
		tk, _ := q.pop()
		got <- tk
	}()

	q.push(queued("a", 1, 1))

	require.Equal(t, "a", receive(t, got).trip.TripID)
}

func TestPriorityQueueClosedQueueIsDrainedThenStopsPop(t *testing.T) {
	q := newPriorityQueue(3)
	q.push(queued("b", 2, 1))
	q.push(queued("a", 1, 2))
	waiting := make(chan bool, 1)

	q.shut()

	require.Equal(t, []string{"a", "b"}, popIDs(t, q, 2))
	go func() {
		_, ok := q.pop()
		waiting <- ok
	}()
	require.False(t, receive(t, waiting), "an empty closed queue releases the waiting worker")
}

func TestPriorityQueueClosingReleasesAllWaitingWorkers(t *testing.T) {
	const workers = 4
	q := newPriorityQueue(1)
	released := make(chan struct{}, workers)
	for range workers {
		go func() {
			_, ok := q.pop()
			if !ok {
				released <- struct{}{}
			}
		}()
	}

	q.shut()

	for range workers {
		receive(t, released)
	}
}

func TestPriorityQueueKeepsEveryTaskUnderConcurrentUse(t *testing.T) {
	const producers, perProducer, capacity, consumers = 4, 200, 10, 3
	q := newPriorityQueue(capacity)
	var mu sync.Mutex
	var nextVersion atomic.Uint64
	var accepted, evicted, popped int

	var consumersDone sync.WaitGroup
	for range consumers {
		consumersDone.Add(1)
		go func() {
			defer consumersDone.Done()
			for {
				if _, ok := q.pop(); !ok {
					return
				}
				mu.Lock()
				popped++
				mu.Unlock()
			}
		}()
	}

	var producersDone sync.WaitGroup
	for p := range producers {
		producersDone.Add(1)
		go func() {
			defer producersDone.Done()
			for i := range perProducer {
				ok, ev := q.push(queued("t", (p*perProducer+i)%50, nextVersion.Add(1)))
				mu.Lock()
				if ok {
					accepted++
				}
				if ev != nil {
					evicted++
				}
				mu.Unlock()
			}
		}()
	}
	producersDone.Wait()
	q.shut()
	consumersDone.Wait()

	require.Equal(t, accepted-evicted, popped, "every accepted task is either popped or was evicted")
	require.Zero(t, q.size())
}
