package qbit

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Minimal qBittorrent WebUI API v2 client
type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

// Torrent entry from /api/v2/torrents/info
type Torrent struct {
	Hash     string `json:"hash"`
	Name     string `json:"name"`
	Tracker  string `json:"tracker"`
	Category string `json:"category"`
}

// File entry from /api/v2/torrents/files; files come in torrent order, so the
// position in the list is the file id used by /torrents/filePrio
type File struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"` // 0 = do not download
}

func NewClient(baseURL, username, password string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		http:     &http.Client{Timeout: 30 * time.Second, Jar: jar},
	}
}

// Authenticates the session; no-op when credentials are not set (WebUI auth bypass)
func (c *Client) Login() error {
	if c.username == "" && c.password == "" {
		return nil
	}
	body, err := c.do("POST", "/api/v2/auth/login", url.Values{"username": {c.username}, "password": {c.password}}, false)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(body)) != "Ok." {
		return fmt.Errorf("qBittorrent login failed: %s", strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *Client) Torrents(category string) ([]Torrent, error) {
	path := "/api/v2/torrents/info"
	if category != "" {
		path += "?category=" + url.QueryEscape(category)
	}
	var torrents []Torrent
	return torrents, c.getJSON(path, &torrents)
}

func (c *Client) Files(hash string) ([]File, error) {
	var files []File
	return files, c.getJSON("/api/v2/torrents/files?hash="+url.QueryEscape(hash), &files)
}

// Renames a file inside a torrent; paths are relative to the torrent's save path
func (c *Client) RenameFile(hash, oldPath, newPath string) error {
	_, err := c.do("POST", "/api/v2/torrents/renameFile", url.Values{
		"hash": {hash}, "oldPath": {oldPath}, "newPath": {newPath},
	}, true)
	return err
}

// Sets the download priority of files (0 = do not download, 1 = normal)
func (c *Client) SetFilePriority(hash string, ids []int, priority int) error {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	_, err := c.do("POST", "/api/v2/torrents/filePrio", url.Values{
		"hash": {hash}, "id": {strings.Join(parts, "|")}, "priority": {strconv.Itoa(priority)},
	}, true)
	return err
}

func (c *Client) getJSON(path string, target interface{}) error {
	body, err := c.do("GET", path, nil, true)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

// Performs a request, logging in again once if the session has expired
func (c *Client) do(method, path string, form url.Values, retry bool) ([]byte, error) {
	var reqBody io.Reader
	if form != nil {
		reqBody = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	// qBittorrent rejects requests whose Referer/Origin does not match its host
	req.Header.Set("Referer", c.baseURL)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode == http.StatusForbidden && retry {
		if err := c.Login(); err != nil {
			return nil, err
		}
		return c.do(method, path, form, false)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("qBittorrent %s %s: HTTP %d %s", method, path, res.StatusCode, msg)
	}
	return body, nil
}
