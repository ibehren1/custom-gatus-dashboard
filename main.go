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
	gatusURL       string
	listenAddr     string
	refreshSeconds int
	historySize    int
	title          string
}

func loadConfig() config {
	return config{
		gatusURL:       envString("GATUS_URL", "http://gatus:8080"),
		listenAddr:     envString("LISTEN_ADDR", ":8080"),
		refreshSeconds: envInt("REFRESH_SECONDS", 15),
		historySize:    envInt("HISTORY_SIZE", 10),
		title:          os.Getenv("TITLE"),
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

func main() {
	cfg := loadConfig()

	upstream, err := url.Parse(cfg.gatusURL)
	if err != nil || upstream.Scheme == "" || upstream.Host == "" {
		log.Fatalf("invalid GATUS_URL %q", cfg.gatusURL)
	}

	client := gatus.NewClient(cfg.gatusURL, cfg.historySize, 5*time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, web.Static, "index.html")
	})
	mux.Handle("GET /dash/static/", http.StripPrefix("/dash/static/", http.FileServerFS(web.Static)))
	mux.HandleFunc("GET /dash/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /dash/api/summary", summaryHandler(cfg, client))
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func summaryHandler(cfg config, client *gatus.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		statuses, fetchedAt, err := client.Statuses(r.Context())
		if err != nil {
			log.Printf("fetch statuses: %v", err)
			http.Error(w, "failed to reach gatus", http.StatusBadGateway)
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
			Groups:         gatus.BuildGroups(statuses),
		})
	}
}
