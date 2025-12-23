package merge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/UDL-TF/TF2Chart/src/internal/config"
)

// Merger renders the merged TF2 content tree according to MergeConfig.
type Merger struct {
	cfg *config.MergeConfig
}

// New creates a Merger from the supplied configuration.
func New(cfg *config.MergeConfig) (*Merger, error) {
	if cfg == nil {
		return nil, errors.New("merge config cannot be nil")
	}
	if err := config.ValidatePath(cfg.BasePath); err != nil {
		return nil, fmt.Errorf("invalid base path: %w", err)
	}
	if err := config.ValidatePath(cfg.TargetBase); err != nil {
		return nil, fmt.Errorf("invalid targetBase: %w", err)
	}
	if err := config.ValidatePath(cfg.TargetContent); err != nil {
		return nil, fmt.Errorf("invalid targetContent: %w", err)
	}
	return &Merger{cfg: cfg}, nil
}

// Run executes a full merge pass.
func (m *Merger) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := mergeTree(m.cfg.BasePath, m.cfg.TargetBase); err != nil {
		return fmt.Errorf("merge base: %w", err)
	}
	for _, ov := range m.cfg.Overlays {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := mergeTree(ov.SourcePath, m.cfg.TargetContent); err != nil {
			return fmt.Errorf("merge overlay %s: %w", ov.Name, err)
		}
	}
	if err := ensureWritablePaths(m.cfg.TargetBase, m.cfg.WritablePaths); err != nil {
		return err
	}
	if err := copyTemplateDirs(m.cfg.CopyTemplates, m.cfg.TargetBase, m.cfg.TargetContent); err != nil {
		return err
	}
	if err := copyWritableTemplates(m.cfg.TargetBase, m.cfg.WritablePaths); err != nil {
		return err
	}
	if err := pruneDanglingSymlinks(m.cfg.TargetBase, m.cfg.TargetContent); err != nil {
		return err
	}
	if m.cfg.Permissions.ApplyDuringMerge {
		mode, err := parseFileMode(m.cfg.Permissions.Mode)
		if err != nil {
			return fmt.Errorf("permissions mode: %w", err)
		}
		if err := applyPermissions(m.cfg.Permissions.ApplyPaths, m.cfg.Permissions.User, m.cfg.Permissions.Group, mode); err != nil {
			return err
		}
	}
	return nil
}

func mergeTree(src, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("merge warning: source %s missing, skipping", src)
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("source %s is not a directory", src)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, dirMode(d))
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		sourcePath := filepath.Join(src, rel)
		if err := os.Symlink(sourcePath, target); err != nil {
			return err
		}
		return nil
	})
}

func ensureWritablePaths(target string, paths []config.WritablePath) error {
	for _, wp := range paths {
		dir := filepath.Join(target, filepath.Clean(wp.Path))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("ensure writable %s: %w", wp.Path, err)
		}
		if wp.HostMount != "" {
			if err := os.MkdirAll(filepath.Join(wp.HostMount, filepath.Clean(wp.Path)), 0o755); err != nil {
				log.Printf("merge warning: unable to prep host mount %s: %v", wp.HostMount, err)
			}
		}
	}
	return nil
}

func copyTemplateDirs(entries []config.CopyTemplate, targetBase, targetContent string) error {
	for _, tpl := range entries {
		src := filepath.Join(tpl.SourceMount, filepath.Clean(tpl.SourcePath))
		var destRoot string
		if tpl.TargetMode == "writable" {
			destRoot = targetContent
		} else {
			destRoot = targetBase
		}
		dest := filepath.Join(destRoot, filepath.Clean(tpl.TargetPath))
		if err := copyDirectory(src, dest, tpl.Clean); err != nil {
			return fmt.Errorf("copy template %s -> %s: %w", src, dest, err)
		}
	}
	return nil
}

func copyWritableTemplates(target string, paths []config.WritablePath) error {
	for _, wp := range paths {
		if wp.Template == nil {
			continue
		}
		src := filepath.Join(wp.Template.SourceMount, filepath.Clean(wp.Template.SourcePath))
		dest := filepath.Join(target, filepath.Clean(wp.Path))
		if err := copyDirectory(src, dest, wp.Template.Clean); err != nil {
			return fmt.Errorf("copy writable template %s -> %s: %w", src, dest, err)
		}
	}
	return nil
}

func copyDirectory(src, dest string, clean bool) error {
	log.Printf("copyDirectory: src=%s dest=%s clean=%v", src, dest, clean)
	info, err := os.Stat(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("merge warning: template source %s missing, skipping", src)
			return nil
		}
		return err
	}
	if clean {
		log.Printf("copyDirectory: removing dest %s", dest)
		if err := os.RemoveAll(dest); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dest, info.Mode().Perm()); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, dirMode(d))
		}
		if d.Type()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			log.Printf("copyDirectory: copying symlink %s -> %s to %s", path, linkTarget, target)
			if err := os.Symlink(linkTarget, target); err != nil {
				return err
			}
			return nil
		}
		log.Printf("copyDirectory: copying file %s to %s", path, target)
		// Remove any existing file or symlink at the target
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("copyDirectory: warning - failed to remove existing target %s: %v", target, err)
		}
		return copyFile(path, target, fileMode(d))
	})
}

func dirMode(d fs.DirEntry) os.FileMode {
	info, err := d.Info()
	if err != nil {
		return 0o755
	}
	return info.Mode().Perm()
}

func fileMode(d fs.DirEntry) os.FileMode {
	info, err := d.Info()
	if err != nil {
		return 0o644
	}
	return info.Mode().Perm()
}

func copyFile(src, dest string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	if err := os.Remove(dest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dstFile, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return nil
}

func pruneDanglingSymlinks(paths ...string) error {
	for _, root := range paths {
		if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.Type()&os.ModeSymlink == 0 {
				return nil
			}
			if _, err := os.Stat(path); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
						return removeErr
					}
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func applyPermissions(paths []string, uid, gid int, mode os.FileMode) error {
	for _, root := range paths {
		if strings.TrimSpace(root) == "" {
			continue
		}
		if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			// Use Lchown to change symlink ownership itself, not the target
			if err := os.Lchown(path, uid, gid); err != nil && !ignorePermError(err) {
				return fmt.Errorf("chown %s: %w", path, err)
			}
			// Only chmod non-symlinks (chmod follows symlinks)
			if d.Type()&os.ModeSymlink == 0 {
				if err := os.Chmod(path, mode); err != nil && !ignorePermError(err) {
					return fmt.Errorf("chmod %s: %w", path, err)
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func ignorePermError(err error) bool {
	if err == nil {
		return true
	}
	return errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EPERM) || errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.EROFS)
}

func parseFileMode(val string) (os.FileMode, error) {
	if strings.TrimSpace(val) == "" {
		return 0o755, nil
	}
	parsed, err := strconv.ParseUint(val, 8, 32)
	if err != nil {
		return 0, err
	}
	return os.FileMode(parsed), nil
}
