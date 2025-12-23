package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/UDL-TF/TF2Chart/src/internal/config"
)

func main() {
	env := flag.String("config-env", "PERMISSIONS_CONFIG", "env var containing permission JSON")
	flag.Parse()

	log.Printf("permissions job starting (configEnv=%s)", *env)
	cfg, err := config.FromEnv[config.PermissionJob](*env)
	if err != nil {
		log.Fatalf("load permission config: %v", err)
	}
	log.Printf("permissions config: path=%s uid=%d gid=%d mode=%s", cfg.Path, cfg.User, cfg.Group, cfg.Mode)

	mode, err := parseMode(cfg.Mode)
	if err != nil {
		log.Fatalf("parse mode: %v", err)
	}

	if err := applyPermissions(context.Background(), cfg.Path, cfg.User, cfg.Group, mode); err != nil {
		log.Fatalf("apply permissions: %v", err)
	}
	log.Printf("permissions fixed for %s", cfg.Path)
}

func applyPermissions(ctx context.Context, root string, uid, gid int, mode fs.FileMode) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Use Lchown to change symlink ownership itself, not the target
		if err := os.Lchown(path, uid, gid); err != nil && !shouldIgnorePermissionError(err) {
			return err
		}
		// Only chmod non-symlinks (chmod follows symlinks)
		if d.Type()&os.ModeSymlink == 0 {
			if err := os.Chmod(path, mode); err != nil && !shouldIgnorePermissionError(err) {
				return err
			}
		}
		return nil
	})
}

func parseMode(val string) (fs.FileMode, error) {
	if val == "" {
		return 0o755, nil
	}
	n, err := strconv.ParseUint(val, 8, 32)
	if err != nil {
		return 0, err
	}
	return fs.FileMode(n), nil
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func shouldIgnorePermissionError(err error) bool {
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EROFS) || errors.Is(err, os.ErrPermission) || errors.Is(err, os.ErrNotExist)
}
