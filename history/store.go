package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultDays = 7
	MaxDays     = 365
	MaxText     = 1 << 20
	MaxBytes    = 16 << 20
	MaxItems    = 3000
)

type Entry struct {
	ID      uint64    `json:"id"`
	Text    string    `json:"text"`
	Copied  time.Time `json:"copied"`
	AppName string    `json:"app_name,omitempty"`
	AppID   string    `json:"app_id,omitempty"`
}
type Store struct {
	Days    int     `json:"days"`
	Entries []Entry `json:"entries"`
	NextID  uint64  `json:"next_id"`
	path    string
}

func Open(path string) (*Store, error) {
	s := &Store{Days: DefaultDays, path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	if s.Days < 1 || s.Days > MaxDays {
		return nil, fmt.Errorf("invalid retention setting: %d", s.Days)
	}
	return s, nil
}

// Commit keeps the previous in-memory state if persistence fails.
func (s *Store) Commit(now time.Time, change func(*Store)) error {
	next := *s
	next.Entries = slices.Clone(s.Entries)
	change(&next)
	next.Prune(now)
	if next.Days == s.Days && next.NextID == s.NextID && slices.Equal(next.Entries, s.Entries) {
		return nil
	}
	if err := next.save(); err != nil {
		return err
	}
	*s = next
	return nil
}

func (s *Store) Prune(now time.Time) {
	cutoff := now.Add(-time.Duration(s.Days) * 24 * time.Hour)
	total := 0
	s.Entries = slices.DeleteFunc(s.Entries, func(e Entry) bool {
		if !e.Copied.After(cutoff) || !Valid(e.Text) || total+len(e.Text) > MaxBytes {
			return true
		}
		total += len(e.Text)
		return false
	})
	if len(s.Entries) > MaxItems {
		s.Entries = slices.Delete(s.Entries, MaxItems, len(s.Entries))
	}
}

func Valid(text string) bool {
	return len(text) > 0 && len(text) <= MaxText && utf8.ValidString(text)
}

func (s *Store) Add(text string, now time.Time) {
	s.AddFrom(text, now, "", "")
}

func (s *Store) AddFrom(text string, now time.Time, appName, appID string) {
	if !Valid(text) {
		return
	}
	s.Entries = slices.DeleteFunc(s.Entries, func(v Entry) bool { return v.Text == text })
	s.NextID++
	s.Entries = slices.Insert(s.Entries, 0, Entry{ID: s.NextID, Text: text, Copied: now, AppName: appName, AppID: appID})
}
func (s *Store) Delete(id uint64) {
	s.Entries = slices.DeleteFunc(s.Entries, func(v Entry) bool { return v.ID == id })
}
func (s *Store) Search(query string) []Entry {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return slices.Clone(s.Entries)
	}
	var result []Entry
	for _, e := range s.Entries {
		text := strings.ToLower(e.Text)
		if slices.ContainsFunc(terms, func(t string) bool { return !strings.Contains(text, t) }) {
			continue
		}
		result = append(result, e)
	}
	return result
}
func (s *Store) save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".history-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = json.NewEncoder(f).Encode(s); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}
