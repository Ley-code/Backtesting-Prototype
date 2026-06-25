package engine

import "container/heap"

// eventHeap is a min-heap of events ordered by Time, then by Type so that for
// the same timestamp a MarketEvent is processed before the OrderEvent it
// spawns, before the FillEvent that follows. This guarantees deterministic,
// time-ordered processing.
type eventHeap []Event

func (h eventHeap) Len() int { return len(h) }
func (h eventHeap) Less(i, j int) bool {
	if h[i].Time.Equal(h[j].Time) {
		return h[i].Type < h[j].Type
	}
	return h[i].Time.Before(h[j].Time)
}
func (h eventHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *eventHeap) Push(x any)   { *h = append(*h, x.(Event)) }
func (h *eventHeap) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	*h = old[:n-1]
	return e
}

// EventQueue is the time-ordered backbone of the engine. It is the structure
// that makes look-ahead bias impossible: events can only be popped in
// non-decreasing time order.
type EventQueue struct {
	h eventHeap
}

func NewEventQueue() *EventQueue {
	q := &EventQueue{}
	heap.Init(&q.h)
	return q
}

func (q *EventQueue) Push(e Event) { heap.Push(&q.h, e) }
func (q *EventQueue) Pop() Event   { return heap.Pop(&q.h).(Event) }
func (q *EventQueue) IsEmpty() bool { return q.h.Len() == 0 }
