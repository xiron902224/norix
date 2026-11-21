package desiredwatch

import (
	"testing"
	"time"
)

func TestHubWakesWaiterOnNotify(t *testing.T) {
	h := New()
	ch, cancel := h.Subscribe("node-a")
	defer cancel()

	done := make(chan struct{})
	go func() {
		time.Sleep(20 * time.Millisecond)
		h.Notify("node-a")
		close(done)
	}()

	select {
	case <-ch:
		// woken
	case <-time.After(2 * time.Second):
		t.Fatal("waiter was not woken by Notify")
	}
	<-done
}

func TestHubNotifyAllWakesEveryNode(t *testing.T) {
	h := New()
	a, cancelA := h.Subscribe("a")
	defer cancelA()
	b, cancelB := h.Subscribe("b")
	defer cancelB()

	h.NotifyAll()

	for _, ch := range []<-chan struct{}{a, b} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatal("NotifyAll did not wake a waiter")
		}
	}
}

func TestHubCancelStopsDelivery(t *testing.T) {
	h := New()
	ch, cancel := h.Subscribe("n")
	cancel()
	h.Notify("n")
	select {
	case <-ch:
		t.Fatal("cancelled waiter should not receive")
	case <-time.After(50 * time.Millisecond):
	}
}
