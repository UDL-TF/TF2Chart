package decompress

import (
	"compress/bzip2"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Git LFS pointer magic header
const gitLFSPointerHeader = "version https://git-lfs.github.com/spec/"

// CacheEntry tracks a decompressed file's state
type CacheEntry struct {
	OriginalPath     string    `json:"originalPath"`     // Path to the .bz2 file
	DecompressedPath string    `json:"decompressedPath"` // Path to decompressed file
	SHA256           string    `json:"sha256"`           // SHA256 of decompressed content
	Size             int64     `json:"size"`             // Size of decompressed file
	Timestamp        time.Time `json:"timestamp"`        // Last decompression time
}

// DecompressionCache tracks all decompressed files
type DecompressionCache struct {
	Entries map[string]*CacheEntry `json:"entries"` // Key: decompressed file path
}

// Decompressor handles .bz2 file decompression and split map reassembly.
type Decompressor struct {
	paths     []string
	cachePath string
	cache     *DecompressionCache
}

// New creates a new Decompressor for the given paths.
func New(paths []string) *Decompressor {
	return NewWithCache(paths, "")
}

// NewWithCache creates a new Decompressor with cache support.
func NewWithCache(paths []string, cachePath string) *Decompressor {
	d := &Decompressor{
		paths:     paths,
		cachePath: cachePath,
		cache:     &DecompressionCache{Entries: make(map[string]*CacheEntry)},
	}

	if cachePath != "" {
		if err := d.loadCache(); err != nil {
			log.Printf("decompressor: failed to load cache from %s: %v (starting fresh)", cachePath, err)
		}
	}

	return d
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
	var totalSkipped int
	var totalRedecompressed int

	for _, scanPath := range d.paths {
		decomp, split, skipped, redecomp, err := d.scanAndDecompress(scanPath)
		if err != nil {
			return fmt.Errorf("decompress path %s: %w", scanPath, err)
		}
		totalDecompressed += decomp
		totalSplitMaps += split
		totalSkipped += skipped
		totalRedecompressed += redecomp
	}

	// Save cache after processing
	if d.cachePath != "" {
		if err := d.saveCache(); err != nil {
			log.Printf("decompressor: warning - failed to save cache: %v", err)
		}
	}

	if totalDecompressed > 0 || totalSplitMaps > 0 || totalSkipped > 0 || totalRedecompressed > 0 {
		log.Printf("decompressor: completed - %d files decompressed, %d split maps reassembled, %d skipped (cached), %d re-decompressed (overwritten)",
			totalDecompressed, totalSplitMaps, totalSkipped, totalRedecompressed)
	}

	return nil
}

func (d *Decompressor) scanAndDecompress(rootPath string) (int, int, int, int, error) {
	info, err := os.Stat(rootPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("decompressor: path %s does not exist, skipping", rootPath)
			return 0, 0, 0, 0, nil
		}
		return 0, 0, 0, 0, fmt.Errorf("stat %s: %w", rootPath, err)
	}

	if !info.IsDir() {
		log.Printf("decompressor: path %s is not a directory, skipping", rootPath)
		return 0, 0, 0, 0, nil
	}

	var fileCount int
	var splitMapCount int
	var skippedCount int
	var redecompressedCount int

	// Read directory entries (non-recursive for efficiency)
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("read dir %s: %w", rootPath, err)
	}

	log.Printf("decompressor: scanning %d entries in %s", len(entries), rootPath)

	for _, entry := range entries {
		path := filepath.Join(rootPath, entry.Name())

		// Check for split map folders
		if entry.IsDir() {
			lowerName := strings.ToLower(entry.Name())
			if strings.HasSuffix(lowerName, ".bsp") || strings.HasSuffix(lowerName, ".bsp.bz2.parts") {
				log.Printf("decompressor: found split map folder: %s", path)
				if err := d.processSplitMapWithCache(path); err != nil {
					log.Printf("decompressor: error processing split map %s: %v", path, err)
					continue
				}
				splitMapCount++
			}
			continue // Skip other directories
		}

		// Check if file ends with .bz2
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".bz2") {
			// Check if this is a previously decompressed file that might be overwritten
			if d.cachePath != "" {
				if needs, reason := d.needsRedecompression(path); needs {
					// Look for the original .bz2 file
					bzipPath := path + ".bz2"
					if _, err := os.Stat(bzipPath); err == nil {
						log.Printf("decompressor: re-decompressing %s (reason: %s)", path, reason)
						if err := d.decompressFileWithCache(bzipPath); err != nil {
							log.Printf("decompressor: error re-decompressing %s: %v", bzipPath, err)
						} else {
							redecompressedCount++
						}
					}
				}
			}
			continue
		}

		log.Printf("decompressor: found bz2 file: %s", path)

		// Check if already decompressed and cached
		outPath := strings.TrimSuffix(path, ".bz2")
		if d.cachePath != "" {
			if needs, reason := d.needsRedecompression(outPath); !needs {
				log.Printf("decompressor: skipping %s (already decompressed and cached)", path)
				skippedCount++
				continue
			} else if reason != "file does not exist" && reason != "not in cache" {
				log.Printf("decompressor: re-decompressing %s (reason: %s)", path, reason)
				redecompressedCount++
			}
		}

		// Decompress the file
		if err := d.decompressFileWithCache(path); err != nil {
			log.Printf("decompressor: error decompressing %s: %v", path, err)
			continue
		}

		fileCount++
	}

	return fileCount, splitMapCount, skippedCount, redecompressedCount, nil
}

// decompressFileWithCache decompresses a .bz2 file and updates the cache
func (d *Decompressor) decompressFileWithCache(bzipPath string) error {
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

	// Update cache
	if d.cachePath != "" {
		if err := d.updateCache(bzipPath, outPath); err != nil {
			log.Printf("decompressor: warning - failed to update cache for %s: %v", outPath, err)
		}
	}

	// Remove the .bz2 file
	if err := os.Remove(bzipPath); err != nil {
		log.Printf("decompressor: warning - failed to remove %s: %v", bzipPath, err)
		// Don't fail the entire operation if we can't remove the source
	} else {
		log.Printf("decompressor: removed %s", bzipPath)
	}

	return nil
}

// processSplitMapWithCache processes split map with cache support
func (d *Decompressor) processSplitMapWithCache(folderPath string) error {
	// Determine output file name first
	folderName := filepath.Base(folderPath)
	outputName := folderName
	if strings.HasSuffix(strings.ToLower(outputName), ".bsp.bz2.parts") {
		outputName = outputName[:len(outputName)-len(".bz2.parts")]
	}
	outputPath := filepath.Join(filepath.Dir(folderPath), outputName)

	// Check cache if enabled
	if d.cachePath != "" {
		if needs, reason := d.needsRedecompression(outputPath); !needs {
			log.Printf("decompressor: skipping split map %s (already assembled and cached)", folderPath)
			return nil
		} else if reason != "file does not exist" && reason != "not in cache" {
			log.Printf("decompressor: re-assembling split map %s (reason: %s)", folderPath, reason)
		}
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

	// The output paths were already determined at the start for cache checking
	// No need to recalculate here
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

	// Update cache
	if d.cachePath != "" {
		if err := d.updateCache(folderPath, outputPath); err != nil {
			log.Printf("decompressor: warning - failed to update cache for %s: %v", outputPath, err)
		}
	}

	return nil
}

// loadCache reads the cache from disk
func (d *Decompressor) loadCache() error {
	if d.cachePath == "" {
		return nil
	}

	data, err := os.ReadFile(d.cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Cache doesn't exist yet, that's fine
		}
		return fmt.Errorf("read cache file: %w", err)
	}

	cache := &DecompressionCache{Entries: make(map[string]*CacheEntry)}
	if err := json.Unmarshal(data, cache); err != nil {
		return fmt.Errorf("unmarshal cache: %w", err)
	}

	d.cache = cache
	log.Printf("decompressor: loaded cache with %d entries from %s", len(cache.Entries), d.cachePath)
	return nil
}

// saveCache writes the cache to disk
func (d *Decompressor) saveCache() error {
	if d.cachePath == "" {
		return nil
	}

	data, err := json.MarshalIndent(d.cache, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	// Ensure cache directory exists
	cacheDir := filepath.Dir(d.cachePath)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}

	if err := os.WriteFile(d.cachePath, data, 0644); err != nil {
		return fmt.Errorf("write cache file: %w", err)
	}

	log.Printf("decompressor: saved cache with %d entries to %s", len(d.cache.Entries), d.cachePath)
	return nil
}

// isGitLFSPointer checks if a file is a git-lfs pointer file
func isGitLFSPointer(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// Read first 200 bytes (git-lfs pointers are small)
	buf := make([]byte, 200)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return false, err
	}

	// Check for git-lfs header
	content := string(buf[:n])
	return strings.HasPrefix(content, gitLFSPointerHeader), nil
}

// calculateSHA256 computes SHA256 hash of a file
func calculateSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// needsRedecompression checks if a file needs to be decompressed based on cache
func (d *Decompressor) needsRedecompression(decompressedPath string) (bool, string) {
	// Check if file exists
	info, err := os.Stat(decompressedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, "file does not exist"
		}
		log.Printf("decompressor: warning - stat error for %s: %v", decompressedPath, err)
		return true, "stat error"
	}

	// Check if it's a git-lfs pointer
	isPointer, err := isGitLFSPointer(decompressedPath)
	if err != nil {
		log.Printf("decompressor: warning - error checking git-lfs pointer for %s: %v", decompressedPath, err)
		return true, "error checking pointer"
	}
	if isPointer {
		return true, "git-lfs pointer detected"
	}

	// Check cache
	entry, exists := d.cache.Entries[decompressedPath]
	if !exists {
		return true, "not in cache"
	}

	// Verify size matches
	if entry.Size != info.Size() {
		return true, fmt.Sprintf("size mismatch (cached: %d, actual: %d)", entry.Size, info.Size())
	}

	// Optionally verify SHA256 (expensive, but thorough)
	// This helps detect if git-sync overwrote with a pointer of similar size
	actualSHA, err := calculateSHA256(decompressedPath)
	if err != nil {
		log.Printf("decompressor: warning - error calculating SHA256 for %s: %v", decompressedPath, err)
		return true, "SHA256 calculation error"
	}

	if actualSHA != entry.SHA256 {
		return true, fmt.Sprintf("SHA256 mismatch (content changed)")
	}

	return false, ""
}

// updateCache adds or updates a cache entry
func (d *Decompressor) updateCache(bzipPath, decompressedPath string) error {
	info, err := os.Stat(decompressedPath)
	if err != nil {
		return fmt.Errorf("stat decompressed file: %w", err)
	}

	sha256Hash, err := calculateSHA256(decompressedPath)
	if err != nil {
		return fmt.Errorf("calculate SHA256: %w", err)
	}

	d.cache.Entries[decompressedPath] = &CacheEntry{
		OriginalPath:     bzipPath,
		DecompressedPath: decompressedPath,
		SHA256:           sha256Hash,
		Size:             info.Size(),
		Timestamp:        time.Now(),
	}

	return nil
}
