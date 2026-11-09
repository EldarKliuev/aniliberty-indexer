package qbit

import (
	"aniliberty-indexer/internal/api"

	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestPlanPriorities(t *testing.T) {
	files := []File{{Priority: 1}, {Priority: 1}, {Priority: 0}, {Priority: 7}}
	skip, enable := PlanPriorities(files, []int{2, 3})
	if !reflect.DeepEqual(skip, []int{0, 1}) || !reflect.DeepEqual(enable, []int{2}) {
		t.Errorf("got skip=%v enable=%v", skip, enable)
	}
}

func TestSelectionsPersistAndMerge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sel.json")
	s := NewSelections(path)
	s.Add("ABCDEF", []int{2})
	s.Add("abcdef", []int{0, 2})

	if got := s.pending()["abcdef"]; !reflect.DeepEqual(got, []int{0, 2}) {
		t.Fatalf("merged selection: got %v", got)
	}
	s.applied("abcdef", []int{0}) // stale: a newer grab added files
	if _, ok := s.pending()["abcdef"]; !ok {
		t.Errorf("selection must stay pending after a stale apply")
	}
	s.applied("abcdef", []int{0, 2})
	if len(s.pending()) != 0 {
		t.Errorf("selection must be applied")
	}

	reloaded := NewSelections(path)
	if got := reloaded.items["abcdef"]; got == nil || !reflect.DeepEqual(got.Files, []int{0, 2}) || got.Pending {
		t.Errorf("reloaded: got %+v", got)
	}

	s.Add("old", []int{1})
	s.expire(time.Now().Add(selectionPendingTTL + time.Hour))
	if _, ok := s.items["old"]; ok {
		t.Errorf("stale pending selection must expire")
	}
	if _, ok := s.items["abcdef"]; !ok {
		t.Errorf("applied selection must be kept")
	}
}

func TestRenamerAppliesSelections(t *testing.T) {
	const (
		pack  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		other = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	names := []string{
		"Ao_no_Hako_2nd_Season_[02].mkv",
		"Ao_no_Hako_2nd_Season_[01].mkv",
		"Ao_no_Hako_2nd_Season_[03].mkv",
		"Ao_no_Hako_2nd_Season_[04].mkv",
		"Ao_no_Hako_2nd_Season_[05].mkv",
	}
	fq := &fakeQbit{
		torrents: []Torrent{{Hash: other, Name: "Something else"}},
		files:    map[string][]string{pack: names, other: {"a.mkv", "b.mkv"}},
		renamed:  map[string]int{},
		calls:    map[string]int{},
	}
	srv := httptest.NewServer(fq)
	defer srv.Close()

	sel := NewSelections("")
	sel.Add(pack, []int{2})
	lookup := func(string) (api.CombinedItem, bool, error) { return api.CombinedItem{}, false, nil }
	r := NewRenamer(NewClient(srv.URL, "", ""), "", lookup, sel)

	r.RunOnce() // torrent is not in qBittorrent yet
	if _, ok := sel.pending()[pack]; !ok {
		t.Fatalf("selection must wait for the torrent")
	}

	fq.mu.Lock()
	fq.torrents = append(fq.torrents, Torrent{Hash: pack, Name: "Ao no Hako 2nd Season - AniLiberty [WEB-DL 1080p]"})
	fq.mu.Unlock()
	r.RunOnce()

	want := map[int]int{0: 0, 1: 0, 3: 0, 4: 0}
	if !reflect.DeepEqual(fq.prio[pack], want) {
		t.Errorf("priorities: got %v, want %v", fq.prio[pack], want)
	}
	if len(fq.prio[other]) != 0 {
		t.Errorf("other torrents must not be touched")
	}
	if len(sel.pending()) != 0 {
		t.Errorf("selection must be applied")
	}

	// A second episode grabbed from the same pack is re-enabled
	sel.Add(pack, []int{3})
	r.RunOnce()
	if fq.prio[pack][3] != 1 || fq.prio[pack][0] != 0 {
		t.Errorf("after second grab: got %v", fq.prio[pack])
	}
}
