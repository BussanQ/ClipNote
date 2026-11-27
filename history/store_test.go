package history

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestHistoryLifecycle(t *testing.T) {
	p := filepath.Join(t.TempDir(), "private", "history.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2000000, 0)
	if s.Days != 7 {
		t.Fatalf("default: %d", s.Days)
	}
	if err = s.Commit(now, func(s *Store) {
		s.Add("Expired", now.Add(-7*24*time.Hour))
		s.Add("   Mixed Case\n原始文本  ", now.Add(-time.Hour))
		s.Add("delete me", now.Add(-time.Minute))
		s.Add("   Mixed Case\n原始文本  ", now)
	}); err != nil {
		t.Fatal(err)
	}
	s, err = Open(p)
	if err != nil {
		t.Fatal(err)
	}
	got := s.Search("CASE 原始")
	if len(s.Entries) != 2 || len(got) != 1 || got[0].Text != "   Mixed Case\n原始文本  " {
		t.Fatalf("bad history: %#v", s.Entries)
	}
	if err = s.Commit(now, func(s *Store) { s.Delete(got[0].ID); s.Days = 12 }); err != nil {
		t.Fatal(err)
	}
	s, err = Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Days != 12 || len(s.Entries) != 1 || s.Entries[0].Text != "delete me" {
		t.Fatalf("bad persisted state: %#v", s)
	}
	stat, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	// Windows inherits the profile directory's ACL instead of POSIX mode bits.
	if runtime.GOOS != "windows" && stat.Mode().Perm() != 0600 {
		t.Fatalf("history permissions: %v", stat.Mode())
	}
}
func TestFailedCommitRetainsPreviousState(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.path = dir
	if s.Commit(time.Now(), func(s *Store) { s.Add("not committed", time.Now()) }) == nil {
		t.Fatal("expected error")
	}
	if len(s.Entries) != 0 || s.NextID != 0 {
		t.Fatal("failed changes escaped")
	}
}
