package ibeamcorelib

import (
	"fmt"
	"testing"
	"time"

	pb "github.com/SKAARHOJ/ibeam-corelib-go/ibeam-core"
	b "github.com/SKAARHOJ/ibeam-corelib-go/paramhelpers"
)

// These tests cover sets that arrive while an earlier set is still waiting for
// device feedback. The manager is driven by hand (no workers) so the test decides
// exactly when the simulated device reports a value.

const setbackControlDelay = 40 * time.Millisecond

type setbackHarness struct {
	t      *testing.T
	m      *IBeamParameterManager
	r      *IBeamParameterRegistry
	pid    uint32
	sent   []int32 // values sent to the device, in order
	errors []pb.ParameterError
}

func newSetbackHarness(t *testing.T, retryCount uint32) *setbackHarness {
	t.Helper()
	manager, registry, _, _ := CreateServer(&pb.CoreInfo{Name: "setbacktest", Description: "test"})
	pid := registry.RegisterParameter(&pb.ParameterDetail{
		Name: "level", Label: "level", ShortLabel: "level", Description: "level", ValueType: pb.ValueType_Integer,
		ControlStyle: pb.ControlStyle_Normal, FeedbackStyle: pb.FeedbackStyle_NormalFeedback,
		RetryCount: retryCount, ControlDelayMs: uint32(setbackControlDelay.Milliseconds()),
		Minimum: 0, Maximum: 100, DefaultValue: b.Int(0),
	})
	if _, err := registry.RegisterDevice(1, 0); err != nil {
		t.Fatal(err)
	}
	manager.serverClientsStream = make(chan *pb.Parameter, 100)

	h := &setbackHarness{t: t, m: manager, r: registry, pid: pid}
	h.report(10)
	h.pump(5 * time.Millisecond)
	h.sent = nil
	return h
}

// set simulates a client set
func (h *setbackHarness) set(v int32) { h.m.ingestTargetParameter(b.Param(h.pid, 1, b.Int(int(v)))) }

// report simulates device feedback
func (h *setbackHarness) report(v int32) {
	h.m.ingestCurrentParameter(b.Param(h.pid, 1, b.Int(int(v))))
}

// pump runs the process loop for d: it handles reevaluations (including the ones
// fired by retry timers) and records what is sent to the device and to clients.
func (h *setbackHarness) pump(d time.Duration) {
	deadline := time.After(d)
	for {
		select {
		case addr := <-h.m.parameterEvent:
			h.m.processParameter(addr)
		case p := <-h.m.out:
			for _, v := range p.Value {
				h.sent = append(h.sent, v.GetInteger())
			}
		case p := <-h.m.serverClientsStream:
			if p.Error != pb.ParameterError_NoError {
				h.errors = append(h.errors, p.Error)
			}
		case <-deadline:
			return
		}
	}
}

// pumpUntilSent runs the process loop until v was sent to the device, or fails after timeout
func (h *setbackHarness) pumpUntilSent(v int32, timeout time.Duration) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(h.sent) != 0 && h.sent[len(h.sent)-1] == v {
			return
		}
		h.pump(2 * time.Millisecond)
	}
	h.t.Errorf("%d was not sent to the device within %v, sent: %v", v, timeout, h.sent)
}

func (h *setbackHarness) buffer() *ibeamParameterValueBuffer {
	h.t.Helper()
	ds := h.r.loadDeviceState(1)
	ds.mu.Lock()
	defer ds.mu.Unlock()
	dim, err := ds.params[h.pid].multiIndex(nil)
	if err != nil {
		h.t.Fatal(err)
	}
	buf, err := dim.getValue()
	if err != nil {
		h.t.Fatal(err)
	}
	return buf
}

// expectSettled checks the buffer ended on v, confirmed by the device, without errors
func (h *setbackHarness) expectSettled(v int32) {
	h.t.Helper()
	buf := h.buffer()
	if got := buf.targetValue.GetInteger(); got != v {
		h.t.Errorf("target is %d, want %d (sent to device: %v)", got, v, h.sent)
	}
	if got := buf.currentValue.GetInteger(); got != v {
		h.t.Errorf("current is %d, want %d (sent to device: %v)", got, v, h.sent)
	}
	if buf.isAssumedState.Load() {
		h.t.Errorf("value is still assumed")
	}
	if len(h.errors) != 0 {
		h.t.Errorf("unexpected errors sent to clients: %v", h.errors)
	}
}

func (h *setbackHarness) expectSentLast(v int32) {
	h.t.Helper()
	if len(h.sent) == 0 || h.sent[len(h.sent)-1] != v {
		h.t.Errorf("last value sent to device should be %d, sent: %v", v, h.sent)
	}
}

// reportLastSent simulates the device confirming the last value it received
func (h *setbackHarness) reportLastSent() {
	h.t.Helper()
	if len(h.sent) == 0 {
		h.t.Fatal("nothing was sent to the device")
	}
	h.report(h.sent[len(h.sent)-1])
}

func forRetryCounts(t *testing.T, f func(t *testing.T, retryCount uint32)) {
	for _, rc := range []uint32{1, 2} {
		t.Run(fmt.Sprintf("RetryCount%d", rc), func(t *testing.T) { f(t, rc) })
	}
}

// A -> B -> A, the set back arrives before the device reported B
func TestSetBackBeforeFeedback(t *testing.T) {
	forRetryCounts(t, func(t *testing.T, rc uint32) {
		h := newSetbackHarness(t, rc)

		h.set(20)
		h.pumpUntilSent(20, 500*time.Millisecond)

		h.set(10)    // set back, B still in flight
		h.report(20) // late feedback for B
		h.pumpUntilSent(10, 500*time.Millisecond)

		h.reportLastSent()
		h.pump(setbackControlDelay * 3)
		h.expectSentLast(10)
		h.expectSettled(10)
	})
}

// A -> B -> A, the device never reports B (it ignored it or merged it away)
func TestSetBackWithoutFeedback(t *testing.T) {
	forRetryCounts(t, func(t *testing.T, rc uint32) {
		h := newSetbackHarness(t, rc)

		h.set(20)
		h.pump(5 * time.Millisecond)
		h.set(10)
		h.pump(setbackControlDelay * 4)

		if len(h.sent) != 1 {
			t.Errorf("expected only the first set to be sent, sent: %v", h.sent)
		}
		h.expectSettled(10)
	})
}

// A -> B -> C, feedback for B arrives before C was sent
func TestNewSetBeforeFeedback(t *testing.T) {
	forRetryCounts(t, func(t *testing.T, rc uint32) {
		h := newSetbackHarness(t, rc)

		h.set(20)
		h.pump(5 * time.Millisecond)
		h.set(30)    // within the control delay, C is not sent yet
		h.report(20) // late feedback for B
		h.pumpUntilSent(30, 500*time.Millisecond)

		h.reportLastSent()
		h.pump(setbackControlDelay * 3)
		h.expectSettled(30)
	})
}

// A change made on the device itself, well after our sets, must still be taken over
func TestExternalChangeAfterSetBack(t *testing.T) {
	forRetryCounts(t, func(t *testing.T, rc uint32) {
		h := newSetbackHarness(t, rc)

		h.set(20)
		h.pump(5 * time.Millisecond)
		h.set(10)
		h.pump(setbackControlDelay * 4)

		h.report(40) // someone changed it on the device
		h.pump(setbackControlDelay * 3)
		if len(h.sent) != 1 {
			t.Errorf("external change should not cause sends, sent: %v", h.sent)
		}
		h.expectSettled(40)
	})
}
