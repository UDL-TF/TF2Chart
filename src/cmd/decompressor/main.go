package main

import (
	"compress/bzip2"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

	log.Printf("scanning paths: %v", pathsToScan)

	var decompressedCount int
	var splitMapCount int
	for _, scanPath := range pathsToScan {
		decomp, split, err := scanAndDecompress(scanPath)
		if err != nil {
			log.Fatalf("failed to process %s: %v", scanPath, err)
		}
		decompressedCount += decomp
		splitMapCount += split
	}

	log.Printf("decompression complete: %d files processed, %d split maps reassembled", decompressedCount, splitMapCount)
}

func scanAndDecompress(rootPath string) (int, int, error) {
	info, err := os.Stat(rootPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("path %s does not exist, skipping", rootPath)
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("stat %s: %w", rootPath, err)
	}

	if !info.IsDir() {
		log.Printf("path %s is not a directory, skipping", rootPath)
		return 0, 0, nil
	}

	var fileCount int
	var splitMapCount int

	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			log.Printf("warning: walk error at %s: %v", path, walkErr)
			return nil // Continue walking
		}

		// Check for split map folders (folders ending with .bsp or .bsp.bz2.parts)
		if info.IsDir() {
			lowerName := strings.ToLower(info.Name())
			if strings.HasSuffix(lowerName, ".bsp") || strings.HasSuffix(lowerName, ".bsp.bz2.parts") {
				log.Printf("found split map folder: %s", path)
				if err := processSplitMap(path); err != nil {
					return fmt.Errorf("process split map %s: %w", path, err)
				}
				splitMapCount++
				return filepath.SkipDir // Don't walk into the split map folder
			}
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if file ends with .bz2
		if !strings.HasSuffix(strings.ToLower(path), ".bz2") {
			return nil
		}

		log.Printf("found bz2 file: %s", path)

		// Decompress the file
		if err := decompressFile(path); err != nil {
			return fmt.Errorf("decompress %s: %w", path, err)
		}

		fileCount++
		return nil
	})

	return fileCount, splitMapCount, err
}

func decompressFile(bzipPath string) error {
	// Open the bzip2 file
	inFile, err := os.Open(bzipPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer inFile.Close()

	// Create decompressed file path (remove .bz2 extension)
	outPath := strings.TrimSuffix(bzipPath, ".bz2")
	if outPath == bzipPath {
		// Fallback in case extension is different case
		outPath = bzipPath[:len(bzipPath)-4]
	}

	log.Printf("decompressing %s -> %s", bzipPath, outPath)

	// Create output file
	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer outFile.Close()

	// Create bzip2 reader
	bzReader := bzip2.NewReader(inFile)

	// Copy decompressed data
	written, err := io.Copy(outFile, bzReader)
	if err != nil {
		outFile.Close()
		os.Remove(outPath) // Clean up partial file
		return fmt.Errorf("decompress: %w", err)
	}

	// Close output file before removing source
	if err := outFile.Close(); err != nil {
		return fmt.Errorf("close output: %w", err)
	}

	log.Printf("decompressed %d bytes", written)

	// Remove the .bz2 file
	if err := os.Remove(bzipPath); err != nil {
		log.Printf("warning: failed to remove %s: %v", bzipPath, err)
		// Don't fail the entire operation if we can't remove the source
	} else {
		log.Printf("removed %s", bzipPath)
	}

	return nil
}

func processSplitMap(folderPath string) error {
	// Read all files in the folder
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return fmt.Errorf("read dir: %w", err)
	}

	// Find all .bz2.part.* files
	var partFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.Contains(name, ".bz2.part.") {
			partFiles = append(partFiles, filepath.Join(folderPath, name))
		}
	}

	if len(partFiles) == 0 {
		log.Printf("warning: no .bz2.part.* files found in %s", folderPath)
		return nil
	}

	// Sort files by name to ensure chronological order
	sort.Strings(partFiles)
	log.Printf("found %d part files in %s", len(partFiles), folderPath)

	// Determine output file name (folder name without path)
	folderName := filepath.Base(folderPath)

	// Handle different folder naming patterns:
	// "map_name.bsp" -> "map_name.bsp"
	// "map_name.bsp.bz2.parts" -> "map_name.bsp"
	outputName := folderName
	if strings.HasSuffix(strings.ToLower(outputName), ".bsp.bz2.parts") {
		outputName = outputName[:len(outputName)-len(".bz2.parts")]
	}

	outputPath := filepath.Join(filepath.Dir(folderPath), outputName)
	tempOutputPath := outputPath + ".tmp"

	log.Printf("assembling split map: %s -> %s", folderPath, outputPath)

	// Create temporary concatenated bz2 file
	concatBz2Path := tempOutputPath + ".bz2"
	concatFile, err := os.Create(concatBz2Path)
	if err != nil {
		return fmt.Errorf("create concat file: %w", err)
	}

	var totalConcatenated int64

	// Concatenate all part files into a single bz2 file
	for i, partPath := range partFiles {
		log.Printf("concatenating part %d/%d: %s", i+1, len(partFiles), filepath.Base(partPath))

		partFile, err := os.Open(partPath)
		if err != nil {
			concatFile.Close()
			os.Remove(concatBz2Path)
			return fmt.Errorf("open part %s: %w", partPath, err)
		}

		written, err := io.Copy(concatFile, partFile)
		partFile.Close()
		if err != nil {
			concatFile.Close()
			os.Remove(concatBz2Path)
			return fmt.Errorf("concatenate part %s: %w", partPath, err)
		}

		totalConcatenated += written
		log.Printf("concatenated %d bytes from part %d", written, i+1)
	}

	// Close concatenated file
	if err := concatFile.Close(); err != nil {
		os.Remove(concatBz2Path)
		return fmt.Errorf("close concat file: %w", err)
	}

	log.Printf("concatenated %d bytes total into temporary bz2 file", totalConcatenated)

	// Now decompress the concatenated bz2 file
	bzFile, err := os.Open(concatBz2Path)
	if err != nil {
		os.Remove(concatBz2Path)
		return fmt.Errorf("open concat bz2: %w", err)
	}
	defer bzFile.Close()

	// Create temporary output file
	outFile, err := os.Create(tempOutputPath)
	if err != nil {
		os.Remove(concatBz2Path)
		return fmt.Errorf("create output: %w", err)
	}
	defer outFile.Close()

	var totalWritten int64

	// Create bzip2 reader and decompress
	bzReader := bzip2.NewReader(bzFile)
	totalWritten, err = io.Copy(outFile, bzReader)
	if err != nil {
		outFile.Close()
		os.Remove(tempOutputPath)
		os.Remove(concatBz2Path)
		return fmt.Errorf("decompress: %w", err)
	}

	// Close files
	bzFile.Close()
	if err := outFile.Close(); err != nil {
		os.Remove(tempOutputPath)
		os.Remove(concatBz2Path)
		return fmt.Errorf("close output: %w", err)
	}

	log.Printf("assembled %s: %d bytes total", outputPath, totalWritten)

	// Clean up the temporary concatenated bz2 file
	os.Remove(concatBz2Path)

	// Remove the folder and all its contents
	if err := os.RemoveAll(folderPath); err != nil {
		os.Remove(tempOutputPath) // Clean up temp file
		return fmt.Errorf("remove folder %s: %w", folderPath, err)
	}
	log.Printf("removed folder %s", folderPath)

	// Rename temp file to final output
	if err := os.Rename(tempOutputPath, outputPath); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	log.Printf("created final output: %s", outputPath)

	return nil
}
