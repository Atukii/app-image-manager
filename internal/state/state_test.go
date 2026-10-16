package state_test

import (
	"errors"
	"testing"
	"time"

	"appimage-manager/internal/state"
)

func TestUpdateIsSeenByALaterLoadAndBumpsTheGeneration(t *testing.T) {
	dir := t.TempDir()

	_, err := state.Open(dir).Update(func(s *state.State) error {
		s.Apps["obsidian"] = state.App{ID: "obsidian", Poll: state.Weekly}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := state.Open(dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Apps["obsidian"].Poll != state.Weekly || got.Generation != 1 {
		t.Fatalf("got %+v; want obsidian polled weekly at generation 1", got)
	}
}

func TestAFailedUpdateWritesNothing(t *testing.T) {
	dir := t.TempDir()

	_, err := state.Open(dir).Update(func(s *state.State) error {
		s.Apps["obsidian"] = state.App{ID: "obsidian"}
		return errors.New("download failed")
	})

	got, _ := state.Open(dir).Load()
	if err == nil || len(got.Apps) != 0 || got.Generation != 0 {
		t.Fatalf("got error %v and state %+v; want the error and an untouched, empty state", err, got)
	}
}

func TestAnUpdateWaitsForTheLockThenGivesUp(t *testing.T) {
	dir := t.TempDir()
	other := state.Open(dir)
	other.LockTimeout = 100 * time.Millisecond
	var otherErr error

	state.Open(dir).Update(func(*state.State) error {
		// Another aim process tries to update while this one holds the lock.
		_, otherErr = other.Update(func(*state.State) error { return nil })
		return nil
	})

	if !errors.Is(otherErr, state.ErrLocked) {
		t.Fatalf("got %v; want ErrLocked", otherErr)
	}
}
