package caddy_airplane_proxy

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
)

type Subscription struct {
	URL       string `json:"url,omitempty"`
	Route     string `json:"route,omitempty"`
	FileName  string `json:"file_name,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

func (s *Subscription) checkExistOrFetch(storeDir string) {
	if checkExist(s.bodyFilePath(storeDir)) {
		return
	}
	s.fetchAndStore(storeDir)
}

func (s *Subscription) fetchAndStore(storeDir string) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", s.URL, nil)
	if err != nil {
		slog.Error("creating request failed", "url", s.URL, "error", err)
		return
	}

	userAgent := s.UserAgent
	if userAgent == "" {
		userAgent = "airplane-proxy"
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		slog.Error("fetching subscription failed", "url", s.URL, "error", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("reading response body failed", "url", s.URL, "error", err)
		return
	}

	rawPath := s.bodyFilePath(storeDir)
	headerPath := s.headerFilePath(storeDir)

	if err := os.MkdirAll(storeDir, 0755); err != nil {
		slog.Error("creating store directory failed", "path", storeDir, "error", err)
		return
	}

	if err := os.WriteFile(rawPath, body, 0644); err != nil {
		slog.Error("writing raw file failed", "path", rawPath, "error", err)
		return
	}

	subUserInfo := resp.Header.Get("subscription-userinfo")
	if err := os.WriteFile(headerPath, []byte(subUserInfo), 0644); err != nil {
		slog.Error("writing header file failed", "path", headerPath, "error", err)
		return
	}

	slog.Info("fetched and stored subscription", "url", s.URL, "raw_path", rawPath, "header_path", headerPath)
}

func (s *Subscription) bodyFilePath(storeDir string) string {
	return filepath.Join(storeDir, s.FileName+".raw")
}

func (s *Subscription) headerFilePath(storeDir string) string {
	return filepath.Join(storeDir, s.FileName+".header")
}
