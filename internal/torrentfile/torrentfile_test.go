package torrentfile

import (
	"os"
	"testing"
)

// Real AniLiberty torrent: five episodes, stored out of order
func TestFilesRealTorrent(t *testing.T) {
	data, err := os.ReadFile("testdata/ao_no_hako_s2.torrent")
	if err != nil {
		t.Fatal(err)
	}
	files, err := Files(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []File{
		{0, "Ao_no_Hako_2nd_Season_[02].mkv", 1666294552},
		{1, "Ao_no_Hako_2nd_Season_[01].mkv", 979334548},
		{2, "Ao_no_Hako_2nd_Season_[03].mkv", 982571891},
		{3, "Ao_no_Hako_2nd_Season_[04].mkv", 983509472},
		{4, "Ao_no_Hako_2nd_Season_[05].mkv", 996599702},
	}
	if len(files) != len(want) {
		t.Fatalf("got %d files, want %d", len(files), len(want))
	}
	for i := range want {
		if files[i] != want[i] {
			t.Errorf("file %d: got %+v, want %+v", i, files[i], want[i])
		}
	}
}

func TestFilesSingleAndNested(t *testing.T) {
	single := "d4:infod6:lengthi42e4:name9:movie.mkv12:piece lengthi16e6:pieces0:ee"
	files, err := Files([]byte(single))
	if err != nil || len(files) != 1 || files[0] != (File{0, "movie.mkv", 42}) {
		t.Errorf("single-file torrent: got %+v, %v", files, err)
	}

	nested := "d4:infod5:filesld6:lengthi5e4:pathl4:Subs9:a_[1].asseee4:name4:Showee"
	files, err = Files([]byte(nested))
	if err != nil || len(files) != 1 || files[0].Path != "Subs/a_[1].ass" {
		t.Errorf("nested path: got %+v, %v", files, err)
	}

	for _, bad := range []string{"", "d4:info", "li1e", "d4:infod4:name1:xee"} {
		if _, err := Files([]byte(bad)); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}
