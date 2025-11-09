package airplane

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

type Subscription struct {
	URL       string `json:"url,omitempty"`
	Route     string `json:"route,omitempty"`
	FileName  string `json:"file_name,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

func (s *Subscription) checkExistOrFetch(subName string, ap *AirplaneProxy) {
	if checkExist(s.bodyFilePath(ap.StoreDir)) {
		return
	}
	ap.fetchAndStore(subName, s)
}

func (ap *AirplaneProxy) fetchAndStore(subName string, s *Subscription) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	req, err := http.NewRequest("GET", s.URL, nil)
	if err != nil {
		ap.logger.Error("creating request", zap.String("sub", subName), zap.Error(err))
		return
	}

	userAgent := s.UserAgent
	if userAgent == "" {
		userAgent = "airplane-proxy"
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		ap.logger.Error("fetching subscription", zap.String("sub", subName), zap.Error(err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		ap.logger.Error("fetching subscription failed", zap.String("sub", subName), zap.Int("status_code", resp.StatusCode))
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ap.logger.Error("reading response body failed", zap.String("sub", subName), zap.Error(err))
		return
	}

	rawPath := s.bodyFilePath(ap.StoreDir)
	headerPath := s.headerFilePath(ap.StoreDir)

	if err := os.MkdirAll(ap.StoreDir, 0755); err != nil {
		ap.logger.Error("creating store directory failed", zap.String("path", ap.StoreDir), zap.Error(err))
		return
	}

	if err := os.WriteFile(rawPath, body, 0644); err != nil {
		ap.logger.Error("writing raw file failed", zap.String("path", rawPath), zap.Error(err))
		return
	}

	subUserInfo := resp.Header.Get("subscription-userinfo")
	if err := os.WriteFile(headerPath, []byte(subUserInfo), 0644); err != nil {
		ap.logger.Error("writing header file failed", zap.String("path", headerPath), zap.Error(err))
		return
	}

	ap.logger.Info("fetched and stored subscription", zap.String("sub", s.Route))
}

func (s *Subscription) bodyFilePath(storeDir string) string {
	return filepath.Join(storeDir, s.FileName)
}

func (s *Subscription) headerFilePath(storeDir string) string {
	return filepath.Join(storeDir, s.FileName+".header")
}
