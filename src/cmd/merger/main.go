package main

import (
	"context"
	"flag"
	"log"
	"syscall"
	"time"

	"github.com/UDL-TF/TF2Chart/src/internal/config"
	"github.com/UDL-TF/TF2Chart/src/internal/merge"
)

func main() {
	env := flag.String("config-env", "MERGER_CONFIG", "environment variable containing merge config JSON")
	flag.Parse()

	// Increase file descriptor limit to handle large directories
	if err := increaseFileDescriptorLimit(); err != nil {
		log.Printf("warning: failed to increase file descriptor limit: %v", err)
	}

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

func increaseFileDescriptorLimit() error {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		return err
	}
	log.Printf("current file descriptor limits: soft=%d hard=%d", rLimit.Cur, rLimit.Max)

	// Try to set soft limit to hard limit (maximum allowed)
	rLimit.Cur = rLimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		return err
	}

	log.Printf("increased file descriptor soft limit to %d", rLimit.Cur)
	return nil
}
