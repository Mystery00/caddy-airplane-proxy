package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

func init() {
	caddy.RegisterModule(&AirplaneProxy{})
	httpcaddyfile.RegisterHandlerDirective("airplane_proxy", parseCaddyfile)
}

// AirplaneProxy is a Caddy module that fetches subscriptions and serves them.
type AirplaneProxy struct {
	StoreDir string `json:"store_dir,omitempty"`
	Cron     string `json:"cron,omitempty"`
	Subs     map[string]*Subscription `json:"subs,omitempty"`
	logger   *zap.Logger
	cron     *cron.Cron
	wg       sync.WaitGroup
}

// Subscription defines a single subscription.
type Subscription struct {
	URL       string `json:"url,omitempty"`
	Route     string `json:"route,omitempty"`
	FileName  string `json:"file_name,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

// CaddyModule returns the Caddy module information.
func (ap *AirplaneProxy) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "caddy.apps.airplane_proxy",
		New: func() caddy.Module { return new(AirplaneProxy) },
	}
}

// Provision sets up the module.
func (ap *AirplaneProxy) Provision(ctx caddy.Context) error {
	ap.logger = ctx.Logger(ap)
	if ap.Subs == nil {
		ap.Subs = make(map[string]*Subscription)
	}
	return nil
}

// Start starts the module.
func (ap *AirplaneProxy) Start() error {
	ap.logger.Info("starting airplane_proxy app")
	ap.cron = cron.New()
	for name, sub := range ap.Subs {
		s := sub
		s.FileName = name
		_, err := ap.cron.AddFunc(ap.Cron, func() {
			ap.wg.Add(1)
			defer ap.wg.Done()
			ap.fetchAndStore(s)
		})
		if err != nil {
			return fmt.Errorf("adding cron job for sub %s: %v", s.FileName, err)
		}
	}
	ap.cron.Start()
	return nil
}

// Stop stops the module.
func (ap *AirplaneProxy) Stop() error {
	ap.logger.Info("stopping airplane_proxy app")
	if ap.cron != nil {
		ap.cron.Stop()
	}
	ap.wg.Wait()
	return nil
}

func (ap *AirplaneProxy) fetchAndStore(sub *Subscription) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", sub.URL, nil)
	if err != nil {
		ap.logger.Error("creating request", zap.String("url", sub.URL), zap.Error(err))
		return
	}

	userAgent := sub.UserAgent
	if userAgent == "" {
		userAgent = "airplane-proxy"
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		ap.logger.Error("fetching subscription", zap.String("url", sub.URL), zap.Error(err))
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		ap.logger.Error("reading response body", zap.String("url", sub.URL), zap.Error(err))
		return
	}

	rawPath := filepath.Join(ap.StoreDir, sub.FileName+".raw")
	headerPath := filepath.Join(ap.StoreDir, sub.FileName+".header")

	if err := os.MkdirAll(ap.StoreDir, 0755); err != nil {
		ap.logger.Error("creating store directory", zap.String("path", ap.StoreDir), zap.Error(err))
		return
	}

	if err := ioutil.WriteFile(rawPath, body, 0644); err != nil {
		ap.logger.Error("writing raw file", zap.String("path", rawPath), zap.Error(err))
		return
	}

	headerContent := ""
	for key, values := range resp.Header {
		for _, value := range values {
			headerContent += fmt.Sprintf("%s: %s\n", key, value)
		}
	}

	if err := ioutil.WriteFile(headerPath, []byte(headerContent), 0644); err != nil {
		ap.logger.Error("writing header file", zap.String("path", headerPath), zap.Error(err))
		return
	}

	ap.logger.Info("fetched and stored subscription", zap.String("url", sub.URL), zap.String("raw_path", rawPath), zap.String("header_path", headerPath))
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (ap *AirplaneProxy) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		for d.NextBlock(0) {
			switch d.Val() {
			case "store_dir":
				if !d.NextArg() {
					return d.ArgErr()
				}
				ap.StoreDir = d.Val()
			case "cron":
				if !d.NextArg() {
					return d.ArgErr()
				}
				ap.Cron = d.Val()
			default:
				subName := d.Val()
				if ap.Subs == nil {
					ap.Subs = make(map[string]*Subscription)
				}
				if _, ok := ap.Subs[subName]; ok {
					return d.Errf("duplicate subscription name: %s", subName)
				}
				sub := new(Subscription)
				ap.Subs[subName] = sub

				for d.NextBlock(1) {
					switch d.Val() {
					case "url":
						if !d.NextArg() {
							return d.ArgErr()
						}
						sub.URL = d.Val()
					case "route":
						if !d.NextArg() {
							return d.ArgErr()
						}
						sub.Route = d.Val()
					case "file_name":
						if !d.NextArg() {
							return d.ArgErr()
						}
						sub.FileName = d.Val()
					case "user_agent":
						if !d.NextArg() {
							return d.ArgErr()
						}
						sub.UserAgent = d.Val()
					default:
						return d.Errf("unrecognized sub-directive: %s", d.Val())
					}
				}
			}
		}
	}

	if ap.Cron == "" {
		return d.Err("cron is required")
	}
	return nil
}

// parseCaddyfile unmarshals tokens from h into a new Middleware.
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var ap AirplaneProxy
	err := ap.UnmarshalCaddyfile(h.Dispenser)
	return ap, err
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (ap AirplaneProxy) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	proxyAppIface, err := r.Context().Value(caddy.AppCtxKey).(*caddy.App).App("airplane_proxy")
	if err != nil {
		return err
	}
	proxyApp := proxyAppIface.(*AirplaneProxy)

	for _, sub := range proxyApp.Subs {
		if r.URL.Path == sub.Route {
			rawPath := filepath.Join(proxyApp.StoreDir, sub.FileName+".raw")
			headerPath := filepath.Join(proxyApp.StoreDir, sub.FileName+".header")

			headerContent, err := ioutil.ReadFile(headerPath)
			if err != nil {
				return caddyhttp.Error(http.StatusInternalServerError, err)
			}

			headers := strings.Split(string(headerContent), "\n")
			for _, header := range headers {
				if parts := strings.SplitN(header, ": ", 2); len(parts) == 2 {
					w.Header().Set(parts[0], parts[1])
				}
			}

			http.ServeFile(w, r, rawPath)
			return nil
		}
	}
	return next.ServeHTTP(w, r)
}
