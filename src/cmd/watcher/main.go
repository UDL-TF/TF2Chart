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

	log.Printf("watcher starting (mergeEnv=%s watcherEnv=%s)", *mergeEnv, *watchEnv)
	mergeCfg, err := config.FromEnv[config.MergeConfig](*mergeEnv)
	if err != nil {
		log.Fatalf("load merge config: %v", err)
	}
	log.Printf("watcher merge config summary: base=%s targetBase=%s targetContent=%s overlays=%d writable=%d copies=%d", mergeCfg.BasePath, mergeCfg.TargetBase, mergeCfg.TargetContent, len(mergeCfg.Overlays), len(mergeCfg.WritablePaths), len(mergeCfg.CopyTemplates))
	merger, err := merge.New(mergeCfg)
	if err != nil {
		log.Fatalf("create merger: %v", err)
	}

	watchCfg, err := config.FromEnv[config.WatcherConfig](*watchEnv)
	if err != nil {
		if !errors.Is(err, config.ErrMissingEnv) {
			log.Fatalf("load watcher config: %v", err)
		}
		log.Printf("watcher config env %s missing, using defaults", *watchEnv)
		watchCfg = &config.WatcherConfig{}
	}
	log.Printf("watch config summary: paths=%d events=%d debounce=%ds poll=%ds", len(watchCfg.WatchPaths), len(watchCfg.Events), watchCfg.DebounceSeconds, watchCfg.PollIntervalSeconds)

	manager, err := watch.NewManager(merger, watchCfg)
	if err != nil {
		log.Fatalf("create watcher manager: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	if err := manager.Run(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			log.Printf("watcher stopped: %v", err)
			return
		}
		log.Fatalf("watcher exited: %v", err)
	}
	log.Printf("watcher stopped cleanly")
}
