// Package state keeps AiM's record of its Managed Apps in apps.json.
package state

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"appimage-manager/internal/desktop"
	"appimage-manager/internal/source"
)

// PollRate is how often a Managed App is Checked.
type PollRate string

const (
	Daily   PollRate = "daily"
	Weekly  PollRate = "weekly"
	Monthly PollRate = "monthly"
	Never   PollRate = "never"
)

// State is everything AiM records.
type State struct {
	// Generation increases with every Update, so a caller can tell whether
	// state changed since it last looked.
	Generation int            `json:"generation"`
	Apps       map[string]App `json:"apps"`
	// Ignored are the paths of Ignored AppImages, which Scans leave out.
	Ignored []string `json:"ignored,omitempty"`
}

// App is AiM's record of one Managed App.
type App struct {
	ID           string     `json:"id"`
	Source       source.Ref `json:"source"`
	AssetPattern string     `json:"assetPattern"`
	// Tag and Published describe the installed Release.
	Tag       string    `json:"tag"`
	Published time.Time `json:"published"`
	Poll      PollRate  `json:"poll"`
	// DesktopFile is the Desktop Entry's filename.
	DesktopFile string            `json:"desktopFile"`
	Overrides   desktop.Overrides `json:"overrides"`
	// IconExt is the icon file's extension in the App Folder.
	IconExt string `json:"iconExt"`
	// UserIcon is set when the user chose the icon, so a Swap keeps it.
	UserIcon bool `json:"userIcon,omitempty"`
	// Previous is the Release of the Previous Version, kept for Rollback.
	Previous *Release `json:"previous,omitempty"`
	// Skipped is the Skipped Release: the tag rolled back from, which Checks
	// ignore until a newer Release appears or the user unskips it.
	Skipped string `json:"skipped,omitempty"`
	// Rejected is a Release tag whose download didn't match its Digest; it is
	// not downloaded again.
	Rejected string `json:"rejected,omitempty"`
	// LastCheck is the outcome of the most recent Check.
	LastCheck *LastCheck `json:"lastCheck,omitempty"`
}

// LastCheck records when a Managed App was last Checked, and how it went.
type LastCheck struct {
	At     time.Time `json:"at"`
	Result string    `json:"result"`
	Error  string    `json:"error,omitempty"`
}

// Release identifies an installed Release.
type Release struct {
	Tag       string    `json:"tag"`
	Published time.Time `json:"published"`
}

// ErrLocked means another aim process held the state lock for longer than
// the Store's LockTimeout.
var ErrLocked = errors.New("another aim process is busy; try again shortly")

// Store reads and writes the state file in a directory.
type Store struct {
	dir string
	// LockTimeout is how long Update waits for another aim process.
	LockTimeout time.Duration
}

// Open returns the Store for dir, typically ~/.local/share/aim.
func Open(dir string) *Store {
	return &Store{dir: dir, LockTimeout: 30 * time.Second}
}

func (s *Store) path() string { return filepath.Join(s.dir, "apps.json") }

// Load reads the current State. A missing file is an empty State.
func (s *Store) Load() (State, error) {
	st := State{Apps: map[string]App{}}
	data, err := os.ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return State{}, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, err
	}
	if st.Apps == nil {
		st.Apps = map[string]App{}
	}
	return st, nil
}

// Update reads the current State, lets change modify it and writes the
// result, increasing the Generation. If change returns an error, nothing is
// written. Update holds an exclusive lock, shared with other aim processes,
// from reading to writing.
func (s *Store) Update(change func(*State) error) (State, error) {
	unlock, err := s.lock()
	if err != nil {
		return State{}, err
	}
	defer unlock()
	st, err := s.Load()
	if err != nil {
		return State{}, err
	}
	if err := change(&st); err != nil {
		return State{}, err
	}
	st.Generation++
	return st, s.write(st)
}

// write replaces the state file atomically.
func (s *Store) write(st State) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".apps-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path())
}

// lock takes the exclusive state lock, waiting up to LockTimeout.
func (s *Store) lock() (unlock func(), err error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(s.dir, "apps.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(s.LockTimeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) || time.Now().After(deadline) {
			f.Close()
			if errors.Is(err, syscall.EWOULDBLOCK) {
				return nil, ErrLocked
			}
			return nil, err
		}
		time.Sleep(25 * time.Millisecond)
	}
}
