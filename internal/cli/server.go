package cli

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/aystro/apod/internal/engine"
	"github.com/aystro/apod/internal/server"
	"github.com/spf13/cobra"
)

var (
	flagListen      string
	flagTLS         bool
	flagTLSCert     string
	flagTLSKey      string
	flagDBPath      string
	flagDataDir     string
	flagDriverDir   string
	flagAcmeEmail   string
	flagTLSMode     string
	flagDNSProvider string
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the apod daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		eng, err := engine.New(engine.Config{
			DBPath:          flagDBPath,
			DataDir:         flagDataDir,
			DriverDir:       flagDriverDir,
			AcmeEmail:       flagAcmeEmail,
			TLSMode:         flagTLSMode,
			ACMEDNSProvider: flagDNSProvider,
		})
		if err != nil {
			return fmt.Errorf("initialize engine: %w", err)
		}
		defer eng.Close()

		srv := server.New(eng)

		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			log.Println("shutting down...")
			eng.Close()
			os.Exit(0)
		}()

		if flagListen != "" {
			go srv.ListenSocket("")
			if flagTLS {
				if flagTLSCert == "" || flagTLSKey == "" {
					return fmt.Errorf("--tls requires --tls-cert and --tls-key")
				}
				return srv.ListenTCPTLS(flagListen, flagTLSCert, flagTLSKey)
			}
			log.Printf("WARNING: serving the API over plaintext HTTP on %s — terminate TLS at a trusted proxy or pass --tls with --tls-cert/--tls-key", flagListen)
			return srv.ListenTCP(flagListen)
		}
		return srv.ListenSocket("")
	},
}

func init() {
	serverCmd.Flags().StringVar(&flagListen, "listen", "", "TCP address to listen on (e.g. 0.0.0.0:8443)")
	serverCmd.Flags().BoolVar(&flagTLS, "tls", false, "Serve the TCP listener over HTTPS (requires --tls-cert and --tls-key)")
	serverCmd.Flags().StringVar(&flagTLSCert, "tls-cert", "", "Path to TLS certificate file (PEM) for --tls")
	serverCmd.Flags().StringVar(&flagTLSKey, "tls-key", "", "Path to TLS private key file (PEM) for --tls")
	serverCmd.Flags().StringVar(&flagDBPath, "db", "", "Database path (default /etc/apod/apod.db)")
	serverCmd.Flags().StringVar(&flagDataDir, "data-dir", "", "Data directory (default /var/lib/apod)")
	serverCmd.Flags().StringVar(&flagDriverDir, "driver-dir", "", "Driver directory (default /etc/apod/drivers)")
	serverCmd.Flags().StringVar(&flagAcmeEmail, "acme-email", "", "Email for Let's Encrypt certificates")
	serverCmd.Flags().StringVar(&flagTLSMode, "tls-mode", "", "TLS strategy: auto (HTTP-01, default), dns (DNS-01), external (proxy-terminated)")
	serverCmd.Flags().StringVar(&flagDNSProvider, "acme-dns-provider", "", "lego DNS provider for --tls-mode=dns (e.g. cloudflare)")
	rootCmd.AddCommand(serverCmd)
}
