package scheduler

import (
	"time"

	"github.com/tbui/yt-studio/domain/entity"
)

// retryItem is a task waiting out its backoff.
type retryItem struct {
	taskID  entity.TaskID
	videoID entity.VideoID
	when    time.Time
}

// retryQueue is a binary min-heap keyed by due time. It exists so the loop can
// arm exactly one timer for the next due retry instead of polling — polling is
// forbidden as a primary mechanism (§8.3).
//
// It is owned by the dispatch goroutine and carries no lock. container/heap
// would need an interface with pointer receivers and boxes every element; a
// dozen lines of sift-up/sift-down here allocate nothing per operation.
type retryQueue struct {
	items []retryItem
}

func (q *retryQueue) len() int { return len(q.items) }

func (q *retryQueue) push(it retryItem) {
	q.items = append(q.items, it)
	i := len(q.items) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if !q.items[i].when.Before(q.items[parent].when) {
			break
		}
		q.items[i], q.items[parent] = q.items[parent], q.items[i]
		i = parent
	}
}

// earliest reports the next due time without removing anything.
func (q *retryQueue) earliest() (time.Time, bool) {
	if len(q.items) == 0 {
		return time.Time{}, false
	}
	return q.items[0].when, true
}

// popDue removes and returns the earliest item if it is due at now.
func (q *retryQueue) popDue(now time.Time) (retryItem, bool) {
	if len(q.items) == 0 || q.items[0].when.After(now) {
		return retryItem{}, false
	}
	top := q.items[0]
	last := len(q.items) - 1
	q.items[0] = q.items[last]
	q.items[last] = retryItem{}
	q.items = q.items[:last]
	q.siftDown(0)
	return top, true
}

func (q *retryQueue) siftDown(i int) {
	n := len(q.items)
	for {
		left, right := 2*i+1, 2*i+2
		smallest := i
		if left < n && q.items[left].when.Before(q.items[smallest].when) {
			smallest = left
		}
		if right < n && q.items[right].when.Before(q.items[smallest].when) {
			smallest = right
		}
		if smallest == i {
			return
		}
		q.items[i], q.items[smallest] = q.items[smallest], q.items[i]
		i = smallest
	}
}
