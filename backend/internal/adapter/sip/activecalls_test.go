package sip

import (
	"context"
	"testing"
)

type fakeByeSender struct {
	byeCalled bool
	byeErr    error
}

func (f *fakeByeSender) Bye(context.Context) error {
	f.byeCalled = true
	return f.byeErr
}

func TestActiveCalls_AddAndCount(t *testing.T) {
	a := NewActiveCalls()
	if got := a.Count(); got != 0 {
		t.Fatalf("Count on empty registry = %d, want 0", got)
	}

	remove1 := a.add("call-1", &fakeByeSender{})
	a.add("call-2", &fakeByeSender{})
	if got := a.Count(); got != 2 {
		t.Fatalf("Count after two adds = %d, want 2", got)
	}

	remove1()
	if got := a.Count(); got != 1 {
		t.Fatalf("Count after removing one = %d, want 1", got)
	}
}

func TestActiveCalls_RemoveIsIdempotent(t *testing.T) {
	a := NewActiveCalls()
	remove := a.add("call-1", &fakeByeSender{})

	remove()
	remove() // must not panic or affect an unrelated later call with the same id

	if got := a.Count(); got != 0 {
		t.Fatalf("Count after double remove = %d, want 0", got)
	}

	a.add("call-1", &fakeByeSender{})
	if got := a.Count(); got != 1 {
		t.Fatalf("Count after re-adding the same id = %d, want 1", got)
	}
}

func TestActiveCalls_SnapshotIsASnapshot(t *testing.T) {
	a := NewActiveCalls()
	a.add("call-1", &fakeByeSender{})

	snap := a.snapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot length = %d, want 1", len(snap))
	}

	a.add("call-2", &fakeByeSender{})
	if len(snap) != 1 {
		t.Fatalf("earlier snapshot mutated after a later add, length = %d, want 1", len(snap))
	}
}
