package main

import (
	"context"
	"flag"
	"log"

	"github.com/UDL-TF/TF2Chart/src/internal/config"
	"github.com/UDL-TF/TF2Chart/src/internal/merge"
)

func main() {
	env := flag.String("config-env", "MERGER_CONFIG", "environment variable containing merge config JSON")
	flag.Parse()

	cfg, err := config.FromEnv[config.MergeConfig](*env)
	if err != nil {
		log.Fatalf("load merge config: %v", err)
	}
	merger, err := merge.New(cfg)
	if err != nil {
		log.Fatalf("create merger: %v", err)
	}
	if err := merger.Run(context.Background()); err != nil {
		log.Fatalf("merge failed: %v", err)
	}
	log.Printf("merge complete")
}
