package cli

import (
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/cederikdotcom/hydraauth"
	"github.com/cederikdotcom/hydramonitor"
	"github.com/cederikdotcom/hydrarelease/internal/api"
	"github.com/cederikdotcom/hydrarelease/internal/store"
	"github.com/cederikdotcom/hydrarelease/pkg/updater"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/acme/autocert"
)

var (
	serveDir          string
	serveDataDir      string
	serveDomain       string
	serveCerts        string
	serveDev          bool
	serveListen       string
	servePublishToken string
	serveAuthToken    string
	serveMirrorURL    string
	serveMirrorToken  string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the release file server",
	RunE: func(cmd *cobra.Command, args []string) error {
		u := updater.NewProductionUpdater("hydrarelease", version)
		u.SetServiceName("hydrarelease")
		u.StartAutoCheck(6*time.Hour, true)
		log.Printf("Auto-update: enabled (every 6h)")

		// Resolve tokens from flags or environment.
		publishToken := servePublishToken
		if publishToken == "" {
			publishToken = os.Getenv("HYDRARELEASE_PUBLISH_TOKEN")
		}

		authToken := serveAuthToken
		if authToken == "" {
			authToken = os.Getenv("HYDRARELEASE_AUTH_TOKEN")
		}
		// Fall back to publish token if no separate auth token.
		if authToken == "" {
			authToken = publishToken
		}

		if authToken == "" {
			log.Printf("Warning: no auth token configured; write endpoints and SSE will be disabled")
		}

		// Initialize stores.
		builds := store.NewBuildStore(serveDataDir)
		releases := store.NewReleaseStore(serveDataDir, serveDir)

		// Initialize auth and monitor.
		auth := hydraauth.New(authToken)
		monitor := hydramonitor.New(hydramonitor.Config{
			AdminToken: authToken,
		})

		startTime := time.Now()

		mirrorURL := serveMirrorURL
		if mirrorURL == "" {
			mirrorURL = os.Getenv("HYDRARELEASE_MIRROR_URL")
		}
		mirrorToken := serveMirrorToken
		if mirrorToken == "" {
			mirrorToken = os.Getenv("HYDRARELEASE_MIRROR_TOKEN")
		}

		srv := &api.Server{
			Builds:      builds,
			Releases:    releases,
			Auth:        auth,
			Monitor:     monitor,
			FileDir:     serveDir,
			Version:     version,
			MirrorURL:   mirrorURL,
			MirrorToken: mirrorToken,
		}

		handler := srv.Handler(publishToken, startTime)

		if serveDev {
			listen := serveListen
			if listen == "" {
				listen = ":8080"
			}
			log.Printf("serving %s on %s (HTTP, dev mode)", serveDir, listen)
			return http.ListenAndServe(listen, handler)
		}

		// Plain HTTP behind reverse proxy.
		if serveListen != "" {
			log.Printf("serving %s on %s (behind reverse proxy, domain=%s)", serveDir, serveListen, serveDomain)
			return http.ListenAndServe(serveListen, handler)
		}

		m := &autocert.Manager{
			Cache:      autocert.DirCache(serveCerts),
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(serveDomain),
		}

		httpSrv := &http.Server{
			Addr:      ":443",
			Handler:   handler,
			TLSConfig: &tls.Config{GetCertificate: m.GetCertificate},
		}

		go func() {
			log.Printf("HTTP redirect server on :80")
			log.Fatal(http.ListenAndServe(":80", m.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "https://"+r.Host+r.URL.String(), http.StatusMovedPermanently)
			}))))
		}()

		log.Printf("serving %s on %s (HTTPS)", serveDir, serveDomain)
		return httpSrv.ListenAndServeTLS("", "")
	},
}

func init() {
	serveCmd.Flags().StringVar(&serveDir, "dir", "/var/www/releases", "directory to serve release files")
	serveCmd.Flags().StringVar(&serveDataDir, "data-dir", "/var/lib/hydrarelease", "directory for build/release metadata")
	serveCmd.Flags().StringVar(&serveDomain, "domain", "releases.experiencenet.com", "domain for TLS certificate")
	serveCmd.Flags().StringVar(&serveCerts, "certs", "/var/lib/hydrarelease/certs", "directory to cache TLS certificates")
	serveCmd.Flags().BoolVar(&serveDev, "dev", false, "run in development mode (plain HTTP)")
	serveCmd.Flags().StringVar(&serveListen, "listen", "", "listen address (default :8080 in dev mode)")
	serveCmd.Flags().StringVar(&servePublishToken, "publish-token", "", "bearer token for legacy publish API (or HYDRARELEASE_PUBLISH_TOKEN env)")
	serveCmd.Flags().StringVar(&serveAuthToken, "auth-token", "", "bearer token for build/release API and SSE (or HYDRARELEASE_AUTH_TOKEN env)")
	serveCmd.Flags().StringVar(&serveMirrorURL, "mirror-url", "", "hydramirror URL to push release files to (or HYDRARELEASE_MIRROR_URL env)")
	serveCmd.Flags().StringVar(&serveMirrorToken, "mirror-token", "", "bearer token for hydramirror (or HYDRARELEASE_MIRROR_TOKEN env)")

	rootCmd.AddCommand(serveCmd)
}
