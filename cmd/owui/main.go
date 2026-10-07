package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/usr1/owui/internal/config"
	"github.com/usr1/owui/internal/store"
	"github.com/usr1/owui/internal/tui"
)

func main() {
	url := flag.String("url", "", "Open WebUI base URL (overrides the config file and OWUI_URL)")
	cfgPath := flag.String("config", "", "config file path (default $XDG_CONFIG_HOME/owui/config.toml)")
	flag.Parse()

	path := config.DefaultPath()
	if *cfgPath != "" {
		path = *cfgPath
	}
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	config.ApplyEnv(&cfg)
	if *url != "" {
		cfg.URL = *url
	}

	index, err := store.Open(store.DefaultPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "chat index: %v\n", err)
		os.Exit(1)
	}

	if err := tui.Run(cfg, index); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
