package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/UDL-TF/TF2Chart/src/internal/config"
	"github.com/UDL-TF/TF2Chart/src/internal/merge"
)

func main() {
	env := flag.String("config-env", "MERGER_CONFIG", "environment variable containing merge config JSON")
	flag.Parse()

	log.Printf("merger starting (configEnv=%s)", *env)
	cfg, err := config.FromEnv[config.MergeConfig](*env)
	if err != nil {
		log.Fatalf("load merge config: %v", err)
	}
	log.Printf("merger config summary: base=%s targetBase=%s targetContent=%s overlays=%d writable=%d copies=%d", cfg.BasePath, cfg.TargetBase, cfg.TargetContent, len(cfg.Overlays), len(cfg.WritablePaths), len(cfg.CopyTemplates))
	merger, err := merge.New(cfg)
	if err != nil {
		log.Fatalf("create merger: %v", err)
	}
	start := time.Now()
	if err := merger.Run(context.Background()); err != nil {
		log.Fatalf("merge failed: %v", err)
	}
	log.Printf("merge complete in %s", time.Since(start))
}
