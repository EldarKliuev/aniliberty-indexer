package qbit

import (
	"aniliberty-indexer/internal/api"
	"aniliberty-indexer/internal/mapping"

	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
)

// File names from the real AniLiberty torrent of Kimetsu no Yaiba: Mugen Ressha-hen (TV)
func kimetsuFiles() []string {
	var files []string
	for i := 1; i <= 7; i++ {
		files = append(files, fmt.Sprintf("Kimetsu no Yaiba Mugen Ressha-hen (TV) - AniLibria.TV [WEBRip 1080p HEVC]/Kimetsu_no_Yaiba_Mugen_Ressha-hen_(TV)_[%02d]_[AniLibria_TV]_[WEBRip_1080p_HEVC].mkv", i))
	}
	return files
}

func TestPlanRenames(t *testing.T) {
	got := PlanRenames(kimetsuFiles(), 2, 0)
	if len(got) != 7 {
		t.Fatalf("expected 7 renames, got %d", len(got))
	}
	want := "Kimetsu no Yaiba Mugen Ressha-hen (TV) - AniLibria.TV [WEBRip 1080p HEVC]/Kimetsu_no_Yaiba_Mugen_Ressha-hen_(TV)_S02E01_[AniLibria_TV]_[WEBRip_1080p_HEVC].mkv"
	if got[0].New != want {
		t.Errorf("got %q, want %q", got[0].New, want)
	}

	tests := []struct {
		name   string
		files  []string
		season int
		offset int
		want   []Rename
	}{
		{"offset is applied", []string{"Show_[01]_[HEVC].mkv"}, 2, 12, []Rename{{"Show_[01]_[HEVC].mkv", "Show_S02E13_[HEVC].mkv"}}},
		{"S01 with offset", []string{"Show_[01]_[HEVC].mkv"}, 1, 12, []Rename{{"Show_[01]_[HEVC].mkv", "Show_S01E13_[HEVC].mkv"}}},
		{"S01 without offset is left alone", []string{"Show_[01]_[HEVC].mkv"}, 1, 0, nil},
		{"already renamed", []string{"Show_S02E01_[HEVC].mkv"}, 2, 0, nil},
		{"non-media files are skipped", []string{"Show_[01].nfo", "cover_[01].jpg"}, 2, 0, nil},
		{"no episode number", []string{"Show_[HEVC].mkv"}, 2, 0, nil},
		{"subtitles are renamed too", []string{"Subs/Show_[03].ass"}, 3, 0, []Rename{{"Subs/Show_[03].ass", "Subs/Show_S03E03.ass"}}},
	}
	for _, tt := range tests {
		if got := PlanRenames(tt.files, tt.season, tt.offset); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// Fake qBittorrent WebUI holding a fixed set of torrents
type fakeQbit struct {
	mu       sync.Mutex
	torrents []Torrent
	files    map[string][]string
	renamed  map[string]int // hash -> rename calls
	calls    map[string]int // hash -> files requests
}

func (f *fakeQbit) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/api/v2/auth/login":
		fmt.Fprint(w, "Ok.")
	case "/api/v2/torrents/info":
		json.NewEncoder(w).Encode(f.torrents)
	case "/api/v2/torrents/files":
		hash := r.URL.Query().Get("hash")
		f.calls[hash]++
		var out []File
		for _, n := range f.files[hash] {
			out = append(out, File{Name: n})
		}
		json.NewEncoder(w).Encode(out)
	case "/api/v2/torrents/renameFile":
		r.ParseForm()
		hash := r.Form.Get("hash")
		for i, n := range f.files[hash] {
			if n == r.Form.Get("oldPath") {
				f.files[hash][i] = r.Form.Get("newPath")
			}
		}
		f.renamed[hash]++
	default:
		http.NotFound(w, r)
	}
}

func TestRenamerOnlyTouchesAniLibertyTVReleases(t *testing.T) {
	if err := mapping.LoadFromBytes([]byte(`[{"mal_id":49926,"tvdb_id":348545,"season":{"tvdb":2}}]`)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mapping.Reset)

	const (
		kimetsu  = "63e8fa5cd94660c77de5e4ea26e3890e65c0513d"
		foreign  = "1111111111111111111111111111111111111111" // other tracker, [01] in names
		fakeName = "2222222222222222222222222222222222222222" // says AniLibria, unknown to AniLiberty
		movie    = "3333333333333333333333333333333333333333"
		s1       = "4444444444444444444444444444444444444444"
	)
	fq := &fakeQbit{
		torrents: []Torrent{
			{Hash: kimetsu, Name: "Kimetsu no Yaiba Mugen Ressha-hen (TV) - AniLibria.TV [WEBRip 1080p HEVC]", Tracker: "http://tr.libria.fun:2710/announce"},
			{Hash: foreign, Name: "[SomeGroup] Show [01-12]", Tracker: "http://nyaa.tracker.wf:7777/announce"},
			{Hash: fakeName, Name: "Show - AniLibria [1080p]"},
			{Hash: movie, Name: "Movie - AniLiberty [BDRip]"},
			{Hash: s1, Name: "Gensou Suikoden - AniLiberty [WEB-DL 1080p HEVC]"},
		},
		files: map[string][]string{
			kimetsu:  kimetsuFiles(),
			foreign:  {"Show/Show_[01].mkv"},
			fakeName: {"Show_[01].mkv"},
			movie:    {"Movie_[01].mkv"},
			s1:       {"Gensou_Suikoden_[01]_[HEVC].mkv"},
		},
		renamed: map[string]int{},
		calls:   map[string]int{},
	}
	srv := httptest.NewServer(fq)
	defer srv.Close()

	lookups := map[string]int{}
	lookup := func(hash string) (api.CombinedItem, bool, error) {
		lookups[hash]++
		var it api.CombinedItem
		switch hash {
		case kimetsu:
			it.Release.Type.Value = "TV"
			it.Release.Mal.ID = 49926
		case movie:
			it.Release.Type.Value = "MOVIE"
		case s1:
			it.Release.Type.Value = "TV"
			it.Release.Name.Latin = "Gensou Suikoden"
		default:
			return it, false, nil
		}
		return it, true, nil
	}

	r := NewRenamer(NewClient(srv.URL, "u", "p"), "", lookup)
	r.RunOnce()
	r.RunOnce() // second pass must not repeat any work

	if fq.renamed[kimetsu] != 7 {
		t.Errorf("kimetsu: expected 7 renames, got %d", fq.renamed[kimetsu])
	}
	if got := fq.files[kimetsu][6]; got != "Kimetsu no Yaiba Mugen Ressha-hen (TV) - AniLibria.TV [WEBRip 1080p HEVC]/Kimetsu_no_Yaiba_Mugen_Ressha-hen_(TV)_S02E07_[AniLibria_TV]_[WEBRip_1080p_HEVC].mkv" {
		t.Errorf("kimetsu: unexpected file name %q", got)
	}
	for _, h := range []string{foreign, fakeName, movie, s1} {
		if fq.renamed[h] != 0 {
			t.Errorf("%s: must not be renamed", h)
		}
	}
	if fq.calls[foreign] != 0 || lookups[foreign] != 0 {
		t.Errorf("foreign torrent must be skipped without any requests")
	}
	for h, n := range lookups {
		if n != 1 {
			t.Errorf("%s: looked up %d times, expected once", h, n)
		}
	}
}

func TestRenamerRetriesWithoutMetadata(t *testing.T) {
	const hash = "5555555555555555555555555555555555555555"
	fq := &fakeQbit{
		torrents: []Torrent{{Hash: hash, Name: "Show - AniLiberty"}},
		files:    map[string][]string{},
		renamed:  map[string]int{},
		calls:    map[string]int{},
	}
	srv := httptest.NewServer(fq)
	defer srv.Close()

	lookup := func(string) (api.CombinedItem, bool, error) {
		var it api.CombinedItem
		it.Release.Type.Value = "TV"
		it.Release.Name.Latin = "Show Season 2"
		return it, true, nil
	}
	r := NewRenamer(NewClient(srv.URL, "", ""), "", lookup)
	r.RunOnce() // magnet without metadata yet

	fq.mu.Lock()
	fq.files[hash] = []string{"Show_[01].mkv"}
	fq.mu.Unlock()
	r.RunOnce()

	if fq.files[hash][0] != "Show_S02E01.mkv" {
		t.Errorf("expected rename after metadata arrived, got %q", fq.files[hash][0])
	}
}
