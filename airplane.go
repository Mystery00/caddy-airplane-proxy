package caddy_airplane_proxy

import (
	"fmt"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/robfig/cron/v3"
	"sync"
)

type AirplaneProxy struct {
	StoreDir string                   `json:"store_dir,omitempty"`
	Cron     string                   `json:"cron,omitempty"`
	Subs     map[string]*Subscription `json:"subs,omitempty"`
	cron     *cron.Cron
	wg       sync.WaitGroup
}

func (ap *AirplaneProxy) validateGlobalOptions() error {
	if ap.StoreDir == "" {
		return fmt.Errorf("store_dir is required")
	}
	for subName, s := range ap.Subs {
		if s.URL == "" {
			return fmt.Errorf("url is required for subscription %s", subName)
		}
		if s.Route == "" {
			return fmt.Errorf("route is required for subscription %s", subName)
		}
		if s.FileName == "" {
			return fmt.Errorf("file_name is required for subscription %s", subName)
		}
	}
	return nil
}

func parseOptions(d *caddyfile.Dispenser, _ any) (any, error) {
	ap := new(AirplaneProxy)
	for d.Next() {
		for d.NextBlock(0) {
			switch d.Val() {
			case "store_dir":
				if !d.NextArg() {
					return nil, d.ArgErr()
				}
				ap.StoreDir = d.Val()
			case "cron":
				if !d.NextArg() {
					return nil, d.ArgErr()
				}
				ap.Cron = d.Val()
			default:
				subName := d.Val()
				if ap.Subs == nil {
					ap.Subs = make(map[string]*Subscription)
				}
				if _, ok := ap.Subs[subName]; ok {
					return nil, d.Errf("duplicate subscription name: %s", subName)
				}
				sub := new(Subscription)
				ap.Subs[subName] = sub

				for d.NextBlock(1) {
					switch d.Val() {
					case "url":
						if !d.NextArg() {
							return nil, d.ArgErr()
						}
						sub.URL = d.Val()
					case "route":
						if !d.NextArg() {
							return nil, d.ArgErr()
						}
						sub.Route = d.Val()
					case "file_name":
						if !d.NextArg() {
							return nil, d.ArgErr()
						}
						sub.FileName = d.Val()
					case "user_agent":
						if !d.NextArg() {
							return nil, d.ArgErr()
						}
						sub.UserAgent = d.Val()
					default:
						return nil, d.Errf("unrecognized sub-directive: %s", d.Val())
					}
				}
			}
		}
	}
	return ap, nil
}

func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	ap := h.Option("airplane_proxy").(*AirplaneProxy)
	return ap, nil
}
