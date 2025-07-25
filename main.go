package airplane

import (
	"fmt"
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/robfig/cron/v3"
	"net/http"
	"os"
)

var (
	_ caddy.Provisioner           = (*AirplaneProxy)(nil)
	_ caddyhttp.MiddlewareHandler = (*AirplaneProxy)(nil)
)

func init() {
	caddy.RegisterModule(&AirplaneProxy{})
	httpcaddyfile.RegisterGlobalOption("airplane_proxy", parseOptions)
	httpcaddyfile.RegisterHandlerDirective("airplane_proxy", parseCaddyfile)
}

func (ap *AirplaneProxy) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.airplane_proxy",
		New: func() caddy.Module { return new(AirplaneProxy) },
	}
}

func (ap *AirplaneProxy) Provision(ctx caddy.Context) error {
	ap.logger = ctx.Logger(ap)
	if ap.Subs == nil {
		ap.Subs = make(map[string]*Subscription)
	}
	return nil
}

func (ap *AirplaneProxy) Start() error {
	err := ap.validateGlobalOptions()
	if err != nil {
		return err
	}
	ap.logger.Info("starting airplane_proxy app")
	if ap.Cron != "" {
		ap.cron = cron.New()
		for subName, sub := range ap.Subs {
			s := sub
			_, err := ap.cron.AddFunc(ap.Cron, func() {
				ap.wg.Add(1)
				defer ap.wg.Done()
				ap.fetchAndStore(subName, s)
			})
			if err != nil {
				return fmt.Errorf("adding cron job for sub %s: %v", s.FileName, err)
			}
		}
		ap.cron.Start()
	} else {
		ap.logger.Info("cron is not set, skip fetching subscriptions periodically")
	}
	for subName, subscription := range ap.Subs {
		subscription.checkExistOrFetch(subName, ap)
	}
	return nil
}

func (ap *AirplaneProxy) Stop() error {
	ap.logger.Info("stopping airplane_proxy app")
	if ap.cron != nil {
		ap.cron.Stop()
	}
	ap.wg.Wait()
	return nil
}

func (ap *AirplaneProxy) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	for _, sub := range ap.Subs {
		if r.URL.Path == sub.Route {
			rawPath := sub.bodyFilePath(ap.StoreDir)
			headerPath := sub.headerFilePath(ap.StoreDir)

			subUserInfo, err := os.ReadFile(headerPath)
			if err != nil {
				return caddyhttp.Error(http.StatusInternalServerError, err)
			}
			if len(subUserInfo) != 0 {
				w.Header().Set("subscription-userinfo", string(subUserInfo))
			}
			http.ServeFile(w, r, rawPath)
			return nil
		}
	}
	return next.ServeHTTP(w, r)
}
