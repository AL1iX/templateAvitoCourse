package positions

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInQueueRejectsDuplicates(t *testing.T) {
	q := newInQueue()

	first, ok := q.add("t1")
	require.True(t, ok)
	_, ok = q.add("t1")
	require.False(t, ok)
	second, ok := q.add("t2")
	require.True(t, ok)

	require.NotEqual(t, first, second)
}

func TestInQueueConcurrentAddAcceptsExactlyOne(t *testing.T) {
	const goroutines = 100
	q := newInQueue()
	start := make(chan struct{})
	var accepted atomic.Int64
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, ok := q.add("t1"); ok {
				accepted.Add(1)
			}
		}()
	}

	close(start)
	wg.Wait()

	require.EqualValues(t, 1, accepted.Load())
}

func TestInQueueCancelRemovesTheEntry(t *testing.T) {
	q := newInQueue()
	version, _ := q.add("t1")

	q.cancel("t1")

	require.False(t, q.current("t1", version))
	_, ok := q.add("t1")
	require.True(t, ok, "a cancelled trip can be queued again")
}

func TestInQueueStaleVersionDoesNotTouchTheNewEntry(t *testing.T) {
	q := newInQueue()
	stale, _ := q.add("t1")
	q.cancel("t1")
	fresh, ok := q.add("t1")
	require.True(t, ok)

	require.False(t, q.current("t1", stale), "a worker that took the old copy must discard it")
	require.True(t, q.current("t1", fresh))

	q.finish("t1", stale)
	require.True(t, q.current("t1", fresh), "finishing a stale copy keeps the new entry")

	q.finish("t1", fresh)
	require.False(t, q.current("t1", fresh))
	_, ok = q.add("t1")
	require.True(t, ok, "a finished trip can be queued again")
}

func TestInQueueConcurrentUseIsRaceFree(t *testing.T) {
	const (
		goroutines = 8
		rounds     = 500
	)
	q := newInQueue()
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range rounds {
				if version, ok := q.add("t1"); ok {
					q.current("t1", version)
					q.finish("t1", version)
				}
				q.cancel("t1")
			}
		}()
	}

	wg.Wait()

	_, ok := q.add("t1")
	require.True(t, ok, "every goroutine ended with a cancel, so nothing is left in the map")
}

func TestInQueueCancelReturnsTheRemovedVersion(t *testing.T) {
	q := newInQueue()
	version, _ := q.add("t1")

	got, existed := q.cancel("t1")
	require.True(t, existed)
	require.Equal(t, version, got)

	_, existed = q.cancel("t1")
	require.False(t, existed, "nothing left to cancel")
}
