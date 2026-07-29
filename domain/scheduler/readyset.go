package scheduler

import "github.com/tbui/yt-studio/domain/entity"

// initialRingCapacity is sized for one 50-chapter video's widest pool (100
// image tasks) so a normal run never grows a ring after warm-up.
const initialRingCapacity = 128

// ring is a fixed-stride FIFO of task pointers. Push and Pop are O(1) and
// allocation-free once the buffer has reached its steady-state capacity, which
// is what lets the dispatch decision hit its zero-allocation budget (§8.3).
type ring struct {
	buf   []*entity.Task
	head  int
	count int
}

func (r *ring) len() int { return r.count }

func (r *ring) push(t *entity.Task) {
	if r.buf == nil {
		r.buf = make([]*entity.Task, initialRingCapacity)
	}
	if r.count == len(r.buf) {
		r.grow()
	}
	idx := r.head + r.count
	if idx >= len(r.buf) {
		idx -= len(r.buf)
	}
	r.buf[idx] = t
	r.count++
}

func (r *ring) grow() {
	next := make([]*entity.Task, len(r.buf)*2)
	n := copy(next, r.buf[r.head:])
	copy(next[n:], r.buf[:r.head])
	r.buf = next
	r.head = 0
}

// peek returns the front entry without removing it.
func (r *ring) peek() *entity.Task {
	if r.count == 0 {
		return nil
	}
	return r.buf[r.head]
}

// pop removes and returns the front entry.
func (r *ring) pop() *entity.Task {
	if r.count == 0 {
		return nil
	}
	t := r.buf[r.head]
	r.buf[r.head] = nil
	r.head++
	if r.head == len(r.buf) {
		r.head = 0
	}
	r.count--
	return t
}

// ReadySet is the authoritative dispatch structure: the scheduler never queries
// SQLite to answer "what can run now?" (§8.3).
//
// It is owned by the dispatch goroutine alone and therefore carries no lock.
// Cancellation and gate changes do not remove entries; they change the task's
// state, and Next drops any entry that is no longer ready. Lazy deletion keeps
// the hot path branch-free of set lookups.
type ReadySet struct {
	queues [entity.NumPools]ring
	// stale counts entries dropped by lazy deletion, for the console.
	stale int
}

// NewReadySet returns an empty ready set with rings preallocated.
func NewReadySet() *ReadySet {
	rs := &ReadySet{}
	for i := range rs.queues {
		rs.queues[i].buf = make([]*entity.Task, initialRingCapacity)
	}
	return rs
}

// Push admits a task that has just become ready.
func (rs *ReadySet) Push(t *entity.Task) {
	i := t.Pool.Index()
	if i < 0 {
		return
	}
	rs.queues[i].push(t)
}

// Next returns the front runnable task of a pool without removing it, dropping
// any entry whose state has moved on since it was pushed. It allocates nothing.
func (rs *ReadySet) Next(pool entity.Pool) *entity.Task {
	i := pool.Index()
	if i < 0 {
		return nil
	}
	q := &rs.queues[i]
	for {
		t := q.peek()
		if t == nil {
			return nil
		}
		if t.State == entity.TaskStateReady {
			return t
		}
		q.pop()
		rs.stale++
	}
}

// Pop removes the front entry of a pool. It is called only after Next returned
// that entry and a slot was acquired for it.
func (rs *ReadySet) Pop(pool entity.Pool) *entity.Task {
	i := pool.Index()
	if i < 0 {
		return nil
	}
	return rs.queues[i].pop()
}

// Len reports the queue depth of one pool, including entries that lazy deletion
// has not yet dropped.
func (rs *ReadySet) Len(pool entity.Pool) int {
	i := pool.Index()
	if i < 0 {
		return 0
	}
	return rs.queues[i].len()
}

// Total reports the queue depth across every pool.
func (rs *ReadySet) Total() int {
	n := 0
	for i := range rs.queues {
		n += rs.queues[i].len()
	}
	return n
}

// Compact drops stale entries from every queue. The dispatch loop calls it
// after a cancellation, so a cancelled video's queued tasks do not linger in
// the depth reported to the operator.
func (rs *ReadySet) Compact() {
	for i := range rs.queues {
		q := &rs.queues[i]
		n := q.len()
		for range n {
			t := q.pop()
			if t != nil && t.State == entity.TaskStateReady {
				q.push(t)
			} else {
				rs.stale++
			}
		}
	}
}
