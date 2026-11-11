package automation

import (
	"testing"
	"time"
)

var testKeys = Keys{Queue: 103, Refresh: 104, Cancel: 105}

func event(t *testing.T, e *Engine, now time.Time, focus bool, timings Timing, expected Kind, up bool) Event {
	t.Helper()
	got, ok := e.Next(now, focus, testKeys, timings)
	if !ok || got.Kind != expected || got.Up != up {
		t.Fatalf("expected %v up=%v, got %+v ok=%v", expected, up, got, ok)
	}
	return got
}
func noEvent(t *testing.T, e *Engine, now time.Time, focus bool, timings Timing) {
	t.Helper()
	if got, ok := e.Next(now, focus, testKeys, timings); ok {
		t.Fatalf("unexpected event %+v", got)
	}
}
func TestOneStartQueuesAndHoldsAKey(t *testing.T) {
	base := time.Unix(100, 0)
	timings := DefaultTiming(800*time.Millisecond, 2400*time.Millisecond)
	var e Engine
	e.Start(base)
	press := event(t, &e, base, true, timings, Queue, false)
	e.Confirm(press, base)
	noEvent(t, &e, base.Add(50*time.Millisecond), true, timings)
	release := event(t, &e, base.Add(150*time.Millisecond), true, timings, Queue, true)
	e.Confirm(release, base.Add(150*time.Millisecond))
	noEvent(t, &e, base.Add(200*time.Millisecond), true, timings)
	press = event(t, &e, base.Add(900*time.Millisecond), true, timings, Queue, false)
	e.Confirm(press, base.Add(900*time.Millisecond))
	// Refresh takes precedence after its interval expires.
	up := event(t, &e, base.Add(1100*time.Millisecond), true, timings, Queue, true)
	e.Confirm(up, base.Add(1100*time.Millisecond))
	refresh := event(t, &e, base.Add(2500*time.Millisecond), true, timings, Refresh, false)
	if refresh.Key != testKeys.Refresh {
		t.Fatal("wrong refresh key")
	}
}
func TestFocusLossAlwaysReleasesAndPauses(t *testing.T) {
	base := time.Unix(100, 0)
	timings := DefaultTiming(800*time.Millisecond, 2400*time.Millisecond)
	var e Engine
	e.Start(base)
	press := event(t, &e, base, true, timings, Queue, false)
	e.Confirm(press, base)
	up := event(t, &e, base.Add(10*time.Millisecond), false, timings, Queue, true)
	e.Confirm(up, base.Add(10*time.Millisecond))
	e.Pause(base.Add(20 * time.Millisecond))
	noEvent(t, &e, base.Add(300*time.Millisecond), false, timings)
	press = event(t, &e, base.Add(600*time.Millisecond), true, timings, Queue, false)
	if press.Key != testKeys.Queue {
		t.Fatal("expected queued TC after returning")
	}
}
func TestStopReleasesEvenBeforeHoldTime(t *testing.T) {
	base := time.Unix(100, 0)
	timings := DefaultTiming(800*time.Millisecond, 2400*time.Millisecond)
	var e Engine
	e.Start(base)
	press := event(t, &e, base, true, timings, Queue, false)
	e.Confirm(press, base)
	e.Stop()
	up := event(t, &e, base.Add(5*time.Millisecond), true, timings, Queue, true)
	e.Confirm(up, base.Add(5*time.Millisecond))
	noEvent(t, &e, base.Add(time.Second), true, timings)
}
func TestRefundCountsCompletedPulsesOnly(t *testing.T) {
	base := time.Unix(100, 0)
	timings := DefaultTiming(800*time.Millisecond, 2400*time.Millisecond)
	var e Engine
	e.StartRefund(base, 2)
	p := event(t, &e, base, true, timings, Refund, false)
	e.Confirm(p, base)
	if e.Remaining() != 2 {
		t.Fatal("decremented before release")
	}
	u := event(t, &e, base.Add(150*time.Millisecond), true, timings, Refund, true)
	e.Confirm(u, base.Add(150*time.Millisecond))
	if e.Remaining() != 1 {
		t.Fatal("did not decrement on release")
	}
	p = event(t, &e, base.Add(300*time.Millisecond), true, timings, Refund, false)
	e.Confirm(p, base.Add(300*time.Millisecond))
	u = event(t, &e, base.Add(450*time.Millisecond), true, timings, Refund, true)
	e.Confirm(u, base.Add(450*time.Millisecond))
	if e.Remaining() != 0 || e.Refunding() {
		t.Fatal("refund did not finish")
	}
}
