package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/UDL-TF/TF2Chart/src/internal/config"
	"github.com/UDL-TF/TF2Chart/src/internal/merge"
	"github.com/UDL-TF/TF2Chart/src/internal/watch"
)

func main() {
	mergeEnv := flag.String("merge-config-env", "MERGER_CONFIG", "env var containing merge JSON")
	watchEnv := flag.String("watcher-config-env", "WATCHER_CONFIG", "env var containing watcher JSON")
	flag.Parse()

	mergeCfg, err := config.FromEnv[config.MergeConfig](*mergeEnv)
	if err != nil {
		log.Fatalf("load merge config: %v", err)
	}
	merger, err := merge.New(mergeCfg)
	if err != nil {
		log.Fatalf("create merger: %v", err)
	}

	watchCfg, err := config.FromEnv[config.WatcherConfig](*watchEnv)
	if err != nil {
		if !errors.Is(err, config.ErrMissingEnv) {
			log.Fatalf("load watcher config: %v", err)
		}
		watchCfg = &config.WatcherConfig{}
	}

	manager, err := watch.NewManager(merger, watchCfg)
	if err != nil {
		log.Fatalf("create watcher manager: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	if err := manager.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("watcher exited: %v", err)
	}
}
