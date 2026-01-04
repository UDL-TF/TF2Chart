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
	paths     []string
	outputDir string // Output directory for decompressed files (preserves structure)
}

// New creates a new Decompressor for the given paths.
func New(paths []string) *Decompressor {
	return NewWithOutputDir(paths, "")
}

// NewWithOutputDir creates a new Decompressor.
// If outputDir is empty, files are decompressed in-place (original behavior).
// If outputDir is set, files are decompressed to outputDir with preserved directory structure.
func NewWithOutputDir(paths []string, outputDir string) *Decompressor {
	return &Decompressor{
		paths:     paths,
		outputDir: outputDir,
	}
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
		log.Printf("decompressor: completed - %d files decompressed, %d split maps reassembled",
			totalDecompressed, totalSplitMaps)
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

	log.Printf("decompressor: scanning %s recursively", rootPath)

	// Walk the directory tree recursively
	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("decompressor: error accessing %s: %v", path, err)
			return nil // Continue walking despite errors
		}

		// Check for split map folders
		if info.IsDir() {
			lowerName := strings.ToLower(info.Name())
			if strings.HasSuffix(lowerName, ".bsp") || strings.HasSuffix(lowerName, ".bsp.bz2.parts") {
				log.Printf("decompressor: found split map folder: %s", path)
				if err := d.processSplitMap(path); err != nil {
					log.Printf("decompressor: error processing split map %s: %v", path, err)
				} else {
					splitMapCount++
				}
				return filepath.SkipDir // Don't descend into split map folders
			}
			return nil // Continue into other directories
		}

		// Check if file ends with .bz2
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".bz2") {
			return nil
		}

		log.Printf("decompressor: found bz2 file: %s", path)

		// Decompress the file
		if err := d.decompressFile(path); err != nil {
			log.Printf("decompressor: error decompressing %s: %v", path, err)
			return nil
		}

		fileCount++
		return nil
	})

	if err != nil {
		return fileCount, splitMapCount, fmt.Errorf("walk %s: %w", rootPath, err)
	}

	return fileCount, splitMapCount, nil
}

// decompressFile decompresses a .bz2 file
func (d *Decompressor) decompressFile(bzipPath string) error {
	// Determine output path first
	var outPath string
	if d.outputDir != "" {
		// Decompress to output directory, preserving structure
		outPath = d.getOutputPath(bzipPath)
	} else {
		// Decompress in-place (remove .bz2 extension)
		outPath = strings.TrimSuffix(bzipPath, ".bz2")
		if outPath == bzipPath {
			// Fallback in case extension is different case
			outPath = bzipPath[:len(bzipPath)-4]
		}
	}

	// Check if decompressed file already exists (caching)
	if _, err := os.Stat(outPath); err == nil {
		log.Printf("decompressor: skipping %s (already decompressed at %s)", bzipPath, outPath)
		return nil
	}

	// Open the bzip2 file
	inFile, err := os.Open(bzipPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer inFile.Close()

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

	// Keep the source .bz2 file - do not delete it
	log.Printf("decompressor: kept source file %s", bzipPath)

	return nil
}

// processSplitMap processes split map
func (d *Decompressor) processSplitMap(folderPath string) error {
	// Determine output file name first
	folderName := filepath.Base(folderPath)
	outputName := folderName
	if strings.HasSuffix(strings.ToLower(outputName), ".bsp.bz2.parts") {
		outputName = outputName[:len(outputName)-len(".bz2.parts")]
	}

	// Determine output path based on outputDir setting
	var outputPath string
	if d.outputDir != "" {
		// Output to cache directory with preserved structure
		outputPath = d.getOutputPath(filepath.Join(folderPath, outputName))
	} else {
		// Output in-place
		outputPath = filepath.Join(filepath.Dir(folderPath), outputName)
	}

	// Check if assembled file already exists (caching)
	if _, err := os.Stat(outputPath); err == nil {
		log.Printf("decompressor: skipping split map %s (already assembled at %s)", folderPath, outputPath)
		return nil
	}

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

	// The output paths were already determined at the start
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

	// Keep the temporary concatenated bz2 file and parts folder - do not delete them
	log.Printf("decompressor: kept concatenated file %s and parts folder %s", concatBz2Path, folderPath)

	// Rename temp file to final output
	if err := os.Rename(tempOutputPath, outputPath); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	log.Printf("decompressor: created final output: %s", outputPath)

	return nil
}

// getOutputPath determines the output path for a decompressed file
// Outputs directly to cache root without preserving directory structure
func (d *Decompressor) getOutputPath(bzipPath string) string {
	// Remove .bz2 extension from filename
	decompressedName := strings.TrimSuffix(filepath.Base(bzipPath), ".bz2")
	
	// Output directly to cache root
	outPath := filepath.Join(d.outputDir, decompressedName)
	
	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		log.Printf("decompressor: warning - failed to create output directory: %v", err)
	}
	
	return outPath
}
