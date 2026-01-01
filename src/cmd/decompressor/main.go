package main

import (
	"flag"
	"log"
	"strings"
	"syscall"

	"github.com/UDL-TF/TF2Chart/src/internal/decompress"
)

func main() {
	basePath := flag.String("base", "/mnt/base", "base path to check for .bz2 files")
	overlayPaths := flag.String("overlays", "", "comma-separated overlay paths to check (e.g., /mnt/overlays/maps,/mnt/overlays/custom)")
	flag.Parse()

	// Increase file descriptor limit to handle large directories
	if err := increaseFileDescriptorLimit(); err != nil {
		log.Printf("warning: failed to increase file descriptor limit: %v", err)
	}

	log.Println("bz2 decompressor starting")

	// Collect all paths to scan
	var pathsToScan []string

	if *basePath != "" {
		pathsToScan = append(pathsToScan, *basePath)
	}

	if *overlayPaths != "" {
		for _, path := range strings.Split(*overlayPaths, ",") {
			trimmed := strings.TrimSpace(path)
			if trimmed != "" {
				pathsToScan = append(pathsToScan, trimmed)
			}
		}
	}

	if len(pathsToScan) == 0 {
		log.Fatal("no paths to scan provided")
	}

	// Create and run decompressor
	decompressor := decompress.New(pathsToScan)
	if err := decompressor.Run(); err != nil {
		log.Fatalf("decompression failed: %v", err)
	}

	log.Println("decompressor completed successfully")
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
