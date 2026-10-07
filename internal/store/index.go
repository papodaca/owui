// Package store records which chats this TUI created.
// Open WebUI has no client field, so the sidebar is a local list keyed by
// server URL and user id. The chats themselves live on the server.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Entry struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt int64  `json:"updated_at"`
}

type Index struct {
	path string
	mu   sync.Mutex
	// keys are server|user
	byUser map[string][]Entry
}

func DefaultPath() string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "owui", "chats.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".local", "state", "owui", "chats.json")
	}
	return filepath.Join(home, ".local", "state", "owui", "chats.json")
}

func Key(baseURL, userID string) string {
	return strings.TrimRight(baseURL, "/") + "|" + userID
}

func Open(path string) (*Index, error) {
	idx := &Index{path: path, byUser: map[string][]Entry{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return idx, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return idx, nil
	}
	if err := json.Unmarshal(b, &idx.byUser); err != nil {
		return nil, err
	}
	if idx.byUser == nil {
		idx.byUser = map[string][]Entry{}
	}
	return idx, nil
}

func (idx *Index) List(key string) []Entry {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	src := idx.byUser[key]
	out := make([]Entry, len(src))
	copy(out, src)
	return out
}

// Put inserts or replaces an entry and moves it to the front.
func (idx *Index) Put(key string, e Entry) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	list := idx.byUser[key]
	next := make([]Entry, 0, len(list)+1)
	next = append(next, e)
	for _, old := range list {
		if old.ID != e.ID {
			next = append(next, old)
		}
	}
	idx.byUser[key] = next
	return idx.saveLocked()
}

func (idx *Index) Rename(key, id, title string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	list := idx.byUser[key]
	for i := range list {
		if list[i].ID == id {
			list[i].Title = title
			idx.byUser[key] = list
			return idx.saveLocked()
		}
	}
	return nil
}

func (idx *Index) Remove(key, id string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	list := idx.byUser[key]
	next := list[:0]
	for _, e := range list {
		if e.ID != id {
			next = append(next, e)
		}
	}
	idx.byUser[key] = next
	return idx.saveLocked()
}

func (idx *Index) Replace(key string, entries []Entry) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.byUser[key] = entries
	return idx.saveLocked()
}

func (idx *Index) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(idx.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(idx.byUser, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := idx.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, idx.path)
}
