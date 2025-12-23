package main

import (
	"errors"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/UDL-TF/TF2Chart/src/internal/config"
)

func main() {
	env := flag.String("config-env", "ENTRYPOINT_CONFIG", "env var containing entrypoint copy JSON")
	flag.Parse()

	cfg, err := config.FromEnv[config.CopyJob](*env)
	if err != nil {
		log.Fatalf("load entrypoint config: %v", err)
	}

	mode, err := parseMode(cfg.Mode)
	if err != nil {
		log.Fatalf("parse chmod: %v", err)
	}

	if err := copyFile(cfg.Source, cfg.Destination, mode); err != nil {
		log.Fatalf("copy entrypoint: %v", err)
	}
	log.Printf("copied %s -> %s", cfg.Source, cfg.Destination)
}

func copyFile(src, dest string, mode os.FileMode) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !srcInfo.Mode().IsRegular() {
		return errors.New("source is not a regular file")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	dstFile, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return dstFile.Chmod(mode)
}

func parseMode(val string) (os.FileMode, error) {
	if val == "" {
		return 0o755, nil
	}
	n, err := strconv.ParseUint(val, 8, 32)
	if err != nil {
		return 0, err
	}
	return os.FileMode(n), nil
}
