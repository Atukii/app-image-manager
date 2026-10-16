package core_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"appimage-manager/internal/testkit"
)

// TestASwapNeverLeavesThePathMissingOrPartial checks ADR 0009's invariants:
// while a Check swaps in a new version, anyone opening the AppImage gets
// either the whole old file or the whole new one, and a file opened before
// the Swap (as a running app's runtime holds it) keeps the old contents.
func TestASwapNeverLeavesThePathMissingOrPartial(t *testing.T) {
	w := newWorld(t)
	addObsidian(t, w, obsidianImage)
	path := w.path("Applications/obsidian/obsidian.AppImage")
	old, _ := os.ReadFile(path)
	updated := w.publish("v1.10.0", oct10, map[string]testkit.AppImage{"Obsidian-1.10.0.AppImage": obsidian110})["Obsidian-1.10.0.AppImage"]
	running, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()

	stop, problem := make(chan struct{}), make(chan error, 1)
	reads := 0
	go func() {
		defer close(problem)
		for {
			select {
			case <-stop:
				return
			default:
			}
			data, err := os.ReadFile(path)
			reads++
			if err != nil {
				problem <- err
				return
			}
			if !bytes.Equal(data, old) && !bytes.Equal(data, updated) {
				problem <- fmt.Errorf("read %d bytes that are neither the old nor the new AppImage", len(data))
				return
			}
		}
	}()
	results, _ := w.core().Check(context.Background(), "obsidian")
	close(stop)
	if err := <-problem; err != nil {
		t.Fatalf("after %d reads during the Swap: %v", reads, err)
	}

	stillOld, _ := io.ReadAll(running)
	if results[0].Result != "installed" || !bytes.Equal(stillOld, old) {
		t.Fatalf("got %+v; want installed, with the file opened before the Swap still reading the old version", results)
	}
}
