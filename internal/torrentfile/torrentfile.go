// Package torrentfile reads the file list of a .torrent (BitTorrent v1) file.
package torrentfile

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// File inside a torrent; Index is its position in the file list, as used by qBittorrent
type File struct {
	Index  int
	Path   string // slash-separated, without the torrent name directory
	Length int64
}

// Returns the files of a .torrent in their original order
func Files(data []byte) ([]File, error) {
	d := &decoder{data: data}
	root, err := d.value()
	if err != nil {
		return nil, err
	}
	top, ok := root.(map[string]interface{})
	if !ok {
		return nil, errors.New("torrent: top level is not a dictionary")
	}
	info, ok := top["info"].(map[string]interface{})
	if !ok {
		return nil, errors.New("torrent: no info dictionary")
	}

	name, _ := info["name"].(string)
	list, multi := info["files"].([]interface{})
	if !multi {
		length, ok := info["length"].(int64)
		if !ok {
			return nil, errors.New("torrent: neither files nor length in info")
		}
		return []File{{Index: 0, Path: name, Length: length}}, nil
	}

	files := make([]File, 0, len(list))
	for i, raw := range list {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("torrent: file %d is not a dictionary", i)
		}
		length, _ := entry["length"].(int64)
		parts, _ := entry["path"].([]interface{})
		segs := make([]string, 0, len(parts))
		for _, p := range parts {
			if s, ok := p.(string); ok {
				segs = append(segs, s)
			}
		}
		files = append(files, File{Index: i, Path: strings.Join(segs, "/"), Length: length})
	}
	return files, nil
}

// Minimal bencode decoder: strings, integers, lists, dictionaries
type decoder struct {
	data []byte
	pos  int
}

func (d *decoder) value() (interface{}, error) {
	if d.pos >= len(d.data) {
		return nil, errors.New("bencode: unexpected end of data")
	}
	switch c := d.data[d.pos]; {
	case c == 'i':
		d.pos++
		end := d.find('e')
		if end < 0 {
			return nil, errors.New("bencode: unterminated integer")
		}
		n, err := strconv.ParseInt(string(d.data[d.pos:end]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bencode: bad integer: %w", err)
		}
		d.pos = end + 1
		return n, nil
	case c == 'l':
		d.pos++
		var list []interface{}
		for {
			if d.pos >= len(d.data) {
				return nil, errors.New("bencode: unterminated list")
			}
			if d.data[d.pos] == 'e' {
				d.pos++
				return list, nil
			}
			v, err := d.value()
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
	case c == 'd':
		d.pos++
		dict := map[string]interface{}{}
		for {
			if d.pos >= len(d.data) {
				return nil, errors.New("bencode: unterminated dictionary")
			}
			if d.data[d.pos] == 'e' {
				d.pos++
				return dict, nil
			}
			k, err := d.str()
			if err != nil {
				return nil, err
			}
			v, err := d.value()
			if err != nil {
				return nil, err
			}
			dict[k] = v
		}
	case c >= '0' && c <= '9':
		return d.str()
	default:
		return nil, fmt.Errorf("bencode: unexpected byte %q at %d", c, d.pos)
	}
}

func (d *decoder) str() (string, error) {
	colon := d.find(':')
	if colon < 0 {
		return "", errors.New("bencode: bad string length")
	}
	n, err := strconv.Atoi(string(d.data[d.pos:colon]))
	if err != nil || n < 0 || colon+1+n > len(d.data) {
		return "", errors.New("bencode: bad string length")
	}
	s := string(d.data[colon+1 : colon+1+n])
	d.pos = colon + 1 + n
	return s, nil
}

func (d *decoder) find(b byte) int {
	for i := d.pos; i < len(d.data); i++ {
		if d.data[i] == b {
			return i
		}
	}
	return -1
}
