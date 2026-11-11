// Package automation plans timed, foreground-only keypresses for the Rust TC tool.
// This package performs no input injection and has no game access.
package automation

import "time"

type Kind uint8

const (
	None Kind = iota
	Queue
	Refresh
	Refund
)

// Event is the next key transition. Up is always needed after a successful down.
type Event struct {
	Key  uint32
	Kind Kind
	Up   bool
}

type Keys struct{ Queue, Refresh, Cancel uint32 }

type Timing struct {
	QueueEvery, RefreshEvery, RefundEvery time.Duration
	HoldFor, GapAfterRelease              time.Duration
}

func DefaultTiming(queue, refresh time.Duration) Timing {
	return Timing{
		QueueEvery: queue, RefreshEvery: refresh, RefundEvery: 180 * time.Millisecond,
		HoldFor: 140 * time.Millisecond, GapAfterRelease: 120 * time.Millisecond,
	}
}

type Engine struct {
	active, refunding bool
	remaining         int
	held              Event
	heldAt            time.Time
	lastEnd           time.Time
	lastQueue         time.Time
	lastRefresh       time.Time
	lastRefund        time.Time
}

func (e *Engine) Start(now time.Time) {
	e.active = true
	e.refunding = false
	e.remaining = 0
	e.lastQueue = time.Time{} // first craft request on the next focused tick
	e.lastRefresh = now
}

func (e *Engine) Stop() { e.active = false; e.refunding = false; e.remaining = 0 }

func (e *Engine) StartRefund(now time.Time, count int) {
	e.active = false
	e.refunding = count > 0
	e.remaining = count
	e.lastRefund = time.Time{}
}

func (e *Engine) Pause(now time.Time) {
	// On returning to Rust, immediately request crafting; give refresh a full interval.
	e.lastQueue = time.Time{}
	e.lastRefresh = now
}

func (e *Engine) Held() bool      { return e.held.Key != 0 }
func (e *Engine) Active() bool    { return e.active }
func (e *Engine) Refunding() bool { return e.refunding }
func (e *Engine) Remaining() int  { return e.remaining }

// Next plans one key-down or key-up. Call Confirm only after Windows accepts
// that specific event. It never presumes that the game accepted the command.
func (e *Engine) Next(now time.Time, focused bool, keys Keys, t Timing) (Event, bool) {
	if e.Held() {
		if !focused || (!e.active && !e.refunding) || now.Sub(e.heldAt) >= t.HoldFor {
			ev := e.held
			ev.Up = true
			return ev, true
		}
		return Event{}, false
	}
	if !focused || (!e.active && !e.refunding) {
		return Event{}, false
	}
	if !e.lastEnd.IsZero() && now.Sub(e.lastEnd) < t.GapAfterRelease {
		return Event{}, false
	}
	if e.refunding && e.remaining > 0 {
		if e.lastRefund.IsZero() || now.Sub(e.lastRefund) >= t.RefundEvery {
			return Event{Key: keys.Cancel, Kind: Refund}, keys.Cancel != 0
		}
		return Event{}, false
	}
	if e.active {
		if now.Sub(e.lastRefresh) >= t.RefreshEvery {
			return Event{Key: keys.Refresh, Kind: Refresh}, keys.Refresh != 0
		}
		if e.lastQueue.IsZero() || now.Sub(e.lastQueue) >= t.QueueEvery {
			return Event{Key: keys.Queue, Kind: Queue}, keys.Queue != 0
		}
	}
	return Event{}, false
}

func (e *Engine) Confirm(ev Event, now time.Time) {
	if !ev.Up {
		e.held = ev
		e.heldAt = now
		switch ev.Kind {
		case Queue:
			e.lastQueue = now
		case Refresh:
			e.lastRefresh = now
		case Refund:
			e.lastRefund = now
		}
		return
	}
	if !e.Held() || e.held.Key != ev.Key {
		return
	}
	e.held = Event{}
	e.lastEnd = now
	if ev.Kind == Refund && e.remaining > 0 {
		e.remaining--
		if e.remaining == 0 {
			e.refunding = false
		}
	}
}
