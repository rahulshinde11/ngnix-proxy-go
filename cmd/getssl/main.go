package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rahulshinde/nginx-proxy-go/internal/acme"
	"github.com/rahulshinde/nginx-proxy-go/internal/ssl"
)

func main() {
	var (
		skipDNSCheck = flag.Bool("skip-dns-check", false, "Do not perform check if DNS points to this machine")
		forceNew     = flag.Bool("new", false, "Override if certificate already exists")
		force        = flag.Bool("force", false, "Do not perform any check and call ACME directly")
		help         = flag.Bool("help", false, "Show help message")
		apiURL       = flag.String("api", "https://acme-v02.api.letsencrypt.org/directory", "ACME API URL")
		sslDir       = flag.String("ssl-dir", "/etc/ssl", "SSL certificate directory")
		challengeDir = flag.String("challenge-dir", "/tmp/acme-challenges", "ACME challenge directory")
	)
	flag.Parse()

	if *help || len(flag.Args()) == 0 {
		printUsage()
		os.Exit(0)
	}

	domains := flag.Args()

	// Override API URL from environment if set
	if envAPI := os.Getenv("LETSENCRYPT_API"); envAPI != "" {
		*apiURL = envAPI
	}

	fmt.Printf("Using Let's Encrypt API URL: %s\n", *apiURL)
	fmt.Printf("SSL Directory: %s\n", *sslDir)
	fmt.Printf("Challenge Directory: %s\n", *challengeDir)
	fmt.Printf("Domains: %v\n", domains)

	// Create ACME manager
	acmeManager := acme.NewManager(*apiURL, *challengeDir)

	for _, domain := range domains {
		fmt.Printf("\n=== Processing domain: %s ===\n", domain)

		certPath := filepath.Join(*sslDir, "certs", domain+".crt")
		keyPath := filepath.Join(*sslDir, "private", domain+".key")
		accountKeyPath := filepath.Join(*sslDir, "accounts", domain+".account.key")

		// Check if certificate already exists
		if !*forceNew && !*force {
			if _, err := os.Stat(certPath); err == nil {
				fmt.Printf("Certificate already exists for %s at %s\n", domain, certPath)
				continue
			}
		}

		// Obtain certificate
		opts := ssl.CertificateOptions{
			Domain:         domain,
			SkipDNSCheck:   *skipDNSCheck,
			ForceNew:       *forceNew,
			Force:          *force,
			CertPath:       certPath,
			KeyPath:        keyPath,
			AccountKeyPath: accountKeyPath,
		}

		if err := obtainCertificate(acmeManager, opts); err != nil {
			fmt.Printf("Failed to obtain certificate for %s: %v\n", domain, err)
			continue
		}

		fmt.Printf("Successfully obtained certificate for %s\n", domain)
		fmt.Printf("Certificate: %s\n", certPath)
		fmt.Printf("Private Key: %s\n", keyPath)
	}

	// Clean up temporary config if created
	tempConfig := "/etc/nginx/conf.d/gen-ssl-direct.conf"
	if _, err := os.Stat(tempConfig); err == nil {
		os.Remove(tempConfig)
	}

	fmt.Println("\nCertificate management completed.")
}

func obtainCertificate(manager *acme.Manager, opts ssl.CertificateOptions) error {
	// Create directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(opts.CertPath), 0755); err != nil {
		return fmt.Errorf("failed to create cert directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(opts.KeyPath), 0755); err != nil {
		return fmt.Errorf("failed to create key directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(opts.AccountKeyPath), 0755); err != nil {
		return fmt.Errorf("failed to create account directory: %v", err)
	}

	// Obtain certificate
	return manager.ObtainCertificate(opts.Domain, opts.CertPath, opts.KeyPath, opts.AccountKeyPath)
}

func printUsage() {
	fmt.Println("Usage: Obtain Let's Encrypt SSL certificate for a domain or multiple domains")
	fmt.Println()
	fmt.Println("       getssl [--options] <hostname1> [hostname2 hostname3 ...]")
	fmt.Println()
	fmt.Println("Available options:")
	fmt.Println("    --skip-dns-check    Do not perform check if DNS points to this machine")
	fmt.Println("    --new              Override if certificate already exists")
	fmt.Println("    --force            Do not perform any check and call ACME directly")
	fmt.Println("    --api=URL          Specify ACME API URL (default: Let's Encrypt production)")
	fmt.Println("    --ssl-dir=DIR      SSL certificate directory (default: /etc/ssl)")
	fmt.Println("    --challenge-dir=DIR ACME challenge directory (default: /tmp/acme-challenges)")
	fmt.Println("    --help             Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("    getssl example.com")
	fmt.Println("    getssl --new example.com www.example.com")
	fmt.Println("    getssl --force --skip-dns-check test.example.com")
}

// dummyLogger is a simple logger implementation for the CLI tool
type dummyLogger struct{}

func (l *dummyLogger) Info(msg string)  { fmt.Println("[INFO]", msg) }
func (l *dummyLogger) Error(msg string) { fmt.Println("[ERROR]", msg) }
func (l *dummyLogger) Debug(msg string) { fmt.Println("[DEBUG]", msg) }
func (l *dummyLogger) Warn(msg string)  { fmt.Println("[WARN]", msg) }
