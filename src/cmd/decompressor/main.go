package main

import (
	"flag"
	"log"
	"strings"

	"github.com/UDL-TF/TF2Chart/src/internal/decompress"
)

func main() {
	basePath := flag.String("base", "/mnt/base", "base path to check for .bz2 files")
	overlayPaths := flag.String("overlays", "", "comma-separated overlay paths to check (e.g., /mnt/overlays/maps,/mnt/overlays/custom)")
	flag.Parse()

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
