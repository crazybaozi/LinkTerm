package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	configPath := flag.String("config", "", "config file path")
	flag.Parse()

	var cfg *Config
	if *configPath != "" {
		var err error
		cfg, err = LoadConfig(*configPath)
		if err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
	} else {
		cfg = DefaultConfig()
		log.Println("no config file specified, using default dev config")
		log.Printf("  data_dir: %s", cfg.Auth.DataDir)
	}

	srv := NewServer(cfg)

	log.Printf("LinkTerm server starting on %s (region=%s)", cfg.Listen, cfg.Region)

	if cfg.TLS.Cert != "" && cfg.TLS.Key != "" {
		log.Printf("TLS enabled: cert=%s key=%s", cfg.TLS.Cert, cfg.TLS.Key)
		if err := http.ListenAndServeTLS(cfg.Listen, cfg.TLS.Cert, cfg.TLS.Key, srv); err != nil {
			log.Fatal(err)
		}
	} else {
		log.Println("TLS disabled (dev mode)")
		if err := http.ListenAndServe(cfg.Listen, srv); err != nil {
			log.Fatal(err)
		}
	}
}
