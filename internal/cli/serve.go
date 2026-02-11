package cli

import (
	"crypto/tls"
	"log"
	"net/http"
	"time"

	"github.com/cederikdotcom/hydrarelease/pkg/updater"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/acme/autocert"
)

var (
	serveDir    string
	serveDomain string
	serveCerts  string
	serveDev    bool
	serveListen string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the release file server",
	RunE: func(cmd *cobra.Command, args []string) error {
		u := updater.NewUpdater("hydrarelease", version)
		u.SetServiceName("hydrarelease")
		u.StartAutoCheck(6*time.Hour, true)
		log.Printf("Auto-update: enabled (every 6h)")

		if serveDev {
			listen := serveListen
			if listen == "" {
				listen = ":8080"
			}
			log.Printf("serving %s on %s (HTTP, dev mode)", serveDir, listen)
			return http.ListenAndServe(listen, http.FileServer(http.Dir(serveDir)))
		}

		m := &autocert.Manager{
			Cache:      autocert.DirCache(serveCerts),
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(serveDomain),
		}

		srv := &http.Server{
			Addr:      ":443",
			Handler:   http.FileServer(http.Dir(serveDir)),
			TLSConfig: &tls.Config{GetCertificate: m.GetCertificate},
		}

		go func() {
			log.Printf("HTTP redirect server on :80")
			log.Fatal(http.ListenAndServe(":80", m.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "https://"+r.Host+r.URL.String(), http.StatusMovedPermanently)
			}))))
		}()

		log.Printf("serving %s on %s (HTTPS)", serveDir, serveDomain)
		return srv.ListenAndServeTLS("", "")
	},
}

func init() {
	serveCmd.Flags().StringVar(&serveDir, "dir", "/var/www/releases", "directory to serve")
	serveCmd.Flags().StringVar(&serveDomain, "domain", "releases.experiencenet.com", "domain for TLS certificate")
	serveCmd.Flags().StringVar(&serveCerts, "certs", "/var/lib/hydrarelease/certs", "directory to cache TLS certificates")
	serveCmd.Flags().BoolVar(&serveDev, "dev", false, "run in development mode (plain HTTP)")
	serveCmd.Flags().StringVar(&serveListen, "listen", "", "listen address for dev mode (default :8080)")

	rootCmd.AddCommand(serveCmd)
}
