package decompress

import (
	"compress/bzip2"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Decompressor handles .bz2 file decompression and split map reassembly.
type Decompressor struct {
	paths []string
}

// New creates a new Decompressor for the given paths.
func New(paths []string) *Decompressor {
	return &Decompressor{paths: paths}
}

// Run scans configured paths for .bz2 files and split maps, then decompresses them.
func (d *Decompressor) Run() error {
	if len(d.paths) == 0 {
		log.Printf("decompressor: no paths configured, skipping")
		return nil
	}

	log.Printf("decompressor: scanning paths: %v", d.paths)

	var totalDecompressed int
	var totalSplitMaps int

	for _, scanPath := range d.paths {
		decomp, split, err := d.scanAndDecompress(scanPath)
		if err != nil {
			return fmt.Errorf("decompress path %s: %w", scanPath, err)
		}
		totalDecompressed += decomp
		totalSplitMaps += split
	}

	if totalDecompressed > 0 || totalSplitMaps > 0 {
		log.Printf("decompressor: completed - %d files decompressed, %d split maps reassembled", totalDecompressed, totalSplitMaps)
	}

	return nil
}

func (d *Decompressor) scanAndDecompress(rootPath string) (int, int, error) {
	info, err := os.Stat(rootPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("decompressor: path %s does not exist, skipping", rootPath)
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("stat %s: %w", rootPath, err)
	}

	if !info.IsDir() {
		log.Printf("decompressor: path %s is not a directory, skipping", rootPath)
		return 0, 0, nil
	}

	var fileCount int
	var splitMapCount int

	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			log.Printf("decompressor: walk error at %s: %v", path, walkErr)
			return nil // Continue walking
		}

		// Check for split map folders (folders ending with .bsp or .bsp.bz2.parts)
		if info.IsDir() {
			lowerName := strings.ToLower(info.Name())
			if strings.HasSuffix(lowerName, ".bsp") || strings.HasSuffix(lowerName, ".bsp.bz2.parts") {
				log.Printf("decompressor: found split map folder: %s", path)
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

		log.Printf("decompressor: found bz2 file: %s", path)

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

	log.Printf("decompressor: decompressing %s -> %s", bzipPath, outPath)

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

	log.Printf("decompressor: decompressed %d bytes", written)

	// Remove the .bz2 file
	if err := os.Remove(bzipPath); err != nil {
		log.Printf("decompressor: warning - failed to remove %s: %v", bzipPath, err)
		// Don't fail the entire operation if we can't remove the source
	} else {
		log.Printf("decompressor: removed %s", bzipPath)
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
		log.Printf("decompressor: warning - no .bz2.part.* files found in %s", folderPath)
		return nil
	}

	// Sort files by name to ensure chronological order
	sort.Strings(partFiles)
	log.Printf("decompressor: found %d part files in %s", len(partFiles), folderPath)

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

	log.Printf("decompressor: assembling split map: %s -> %s", folderPath, outputPath)

	// Create temporary concatenated bz2 file
	concatBz2Path := tempOutputPath + ".bz2"
	concatFile, err := os.Create(concatBz2Path)
	if err != nil {
		return fmt.Errorf("create concat file: %w", err)
	}

	var totalConcatenated int64

	// Concatenate all part files into a single bz2 file
	for i, partPath := range partFiles {
		log.Printf("decompressor: concatenating part %d/%d: %s", i+1, len(partFiles), filepath.Base(partPath))

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
		log.Printf("decompressor: concatenated %d bytes from part %d", written, i+1)
	}

	// Close concatenated file
	if err := concatFile.Close(); err != nil {
		os.Remove(concatBz2Path)
		return fmt.Errorf("close concat file: %w", err)
	}

	log.Printf("decompressor: concatenated %d bytes total into temporary bz2 file", totalConcatenated)

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

	log.Printf("decompressor: assembled %s: %d bytes total", outputPath, totalWritten)

	// Clean up the temporary concatenated bz2 file
	os.Remove(concatBz2Path)

	// Remove the folder and all its contents
	if err := os.RemoveAll(folderPath); err != nil {
		os.Remove(tempOutputPath) // Clean up temp file
		return fmt.Errorf("remove folder %s: %w", folderPath, err)
	}
	log.Printf("decompressor: removed folder %s", folderPath)

	// Rename temp file to final output
	if err := os.Rename(tempOutputPath, outputPath); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	log.Printf("decompressor: created final output: %s", outputPath)

	return nil
}
