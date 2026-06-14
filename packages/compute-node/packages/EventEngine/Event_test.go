package EventEngine

import (
	"testing"
)

func TestNewEvent(t *testing.T) {
	t.Parallel()

	ev := NewEvent("user.login", "alice")
	if ev.GetName() != "user.login" {
		t.Errorf("GetName() = %q, want %q", ev.GetName(), "user.login")
	}
	if data, ok := ev.GetData().(string); !ok || data != "alice" {
		t.Errorf("GetData() = %v, want %v", ev.GetData(), "alice")
	}
}

func TestNewEventNoData(t *testing.T) {
	t.Parallel()

	ev := NewEvent("empty")
	if ev.GetName() != "empty" {
		t.Errorf("GetName() = %q, want %q", ev.GetName(), "empty")
	}
	if ev.GetData() != nil {
		t.Errorf("GetData() = %v, want nil", ev.GetData())
	}
}

func TestNewEventMultipleData(t *testing.T) {
	t.Parallel()

	// Only first data is used
	ev := NewEvent("multi", "first", "second", "third")
	if ev.GetData() != "first" {
		t.Errorf("GetData() = %v, want %q", ev.GetData(), "first")
	}
}

func TestEventEquals(t *testing.T) {
	t.Parallel()

	ev1 := NewEvent("x", 1)
	ev2 := NewEvent("x", 2)
	ev3 := NewEvent("y", 1)

	if !ev1.Equals(ev2) {
		t.Error("ev1.Equals(ev2) should be true (same name)")
	}
	if ev1.Equals(ev3) {
		t.Error("ev1.Equals(ev3) should be false (different name)")
	}
	if !ev1.Equals(ev1) {
		t.Error("ev1.Equals(ev1) should be true (same object)")
	}
	if ev1.Equals(nil) {
		t.Error("ev1.Equals(nil) should be false")
	}
}

func TestEventString(t *testing.T) {
	t.Parallel()

	ev := NewEvent("test", "payload")
	expected := `Event{name="test", data=payload}`
	if s := ev.String(); s != expected {
		t.Errorf("String() = %q, want %q", s, expected)
	}

	evNil := NewEvent("empty")
	expectedNil := `Event{name="empty", data=<nil>}`
	if s := evNil.String(); s != expectedNil {
		t.Errorf("String() = %q, want %q", s, expectedNil)
	}
}
