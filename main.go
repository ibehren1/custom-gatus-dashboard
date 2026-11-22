// Command custom-gatus-dashboard serves a compact pill-style status dashboard
// for Gatus and reverse-proxies the rest of the Gatus UI on the same origin.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ibehren1/custom-gatus-dashboard/internal/gatus"
	"github.com/ibehren1/custom-gatus-dashboard/web"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

type config struct {
	gatusURL         string
	listenAddr       string
	refreshSeconds   int
	pollSeconds      int
	historySize      int
	failureLogSize   int
	failureRetention time.Duration
	theme            string
	title            string
}

func loadConfig() config {
	return config{
		gatusURL:         envString("GATUS_URL", "http://gatus:8080"),
		listenAddr:       envString("LISTEN_ADDR", ":8080"),
		refreshSeconds:   envInt("REFRESH_SECONDS", 15),
		pollSeconds:      envInt("POLL_SECONDS", 10),
		historySize:      envInt("HISTORY_SIZE", 10),
		failureLogSize:   envInt("FAILURE_LOG_SIZE", 100),
		failureRetention: envDuration("FAILURE_RETENTION", 24*time.Hour),
		theme:            envString("GATUS_THEME", "dark"),
		title:            os.Getenv("TITLE"),
	}
}

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		log.Printf("invalid %s=%q, using %d", key, v, fallback)
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		log.Printf("invalid %s=%q, using %s", key, v, fallback)
		return fallback
	}
	return d
}

func main() {
	cfg := loadConfig()

	upstream, err := url.Parse(cfg.gatusURL)
	if err != nil || upstream.Scheme == "" || upstream.Host == "" {
		log.Fatalf("invalid GATUS_URL %q", cfg.gatusURL)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := gatus.NewClient(cfg.gatusURL)
	failures := gatus.NewFailureLog(cfg.failureLogSize, cfg.failureRetention)
	poller := gatus.NewPoller(client, time.Duration(cfg.pollSeconds)*time.Second, cfg.historySize, failures)
	go poller.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		setThemeCookie(w, cfg.theme)
		http.ServeFileFS(w, r, web.Static, "index.html")
	})
	mux.Handle("GET /dash/static/", http.StripPrefix("/dash/static/", http.FileServerFS(web.Static)))
	mux.HandleFunc("GET /dash/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /dash/api/summary", summaryHandler(cfg, client, poller))
	mux.Handle("/", &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(upstream)
			r.SetXForwarded()
		},
	})

	srv := &http.Server{
		Addr:              cfg.listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("custom-gatus-dashboard %s listening on %s, gatus at %s", version, cfg.listenAddr, cfg.gatusURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func summaryHandler(cfg config, client *gatus.Client, poller *gatus.Poller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groups, failures, fetchedAt, stale, ok := poller.Snapshot()
		if !ok {
			http.Error(w, "no data from gatus yet", http.StatusBadGateway)
			return
		}

		title := cfg.title
		if title == "" {
			title = client.Title(r.Context())
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(gatus.Summary{
			Title:          title,
			Version:        version,
			RefreshSeconds: cfg.refreshSeconds,
			Updated:        fetchedAt,
			Stale:          stale,
			Groups:         groups,
			Failures:       failures,

			FailureRetentionSeconds: int(cfg.failureRetention.Seconds()),
		})
	}
}

// setThemeCookie sets the cookie the Gatus UI reads its light/dark theme from,
// so endpoint detail pages opened from the dashboard match it. "none" leaves
// the user's Gatus theme choice alone.
func setThemeCookie(w http.ResponseWriter, theme string) {
	if theme != "dark" && theme != "light" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "theme",
		Value:    theme,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		SameSite: http.SameSiteStrictMode,
	})
}
