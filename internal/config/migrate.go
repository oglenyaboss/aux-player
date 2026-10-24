package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// MigrateLegacy moves data the app wrote under its old vibez name into the aux dirs.
// It runs once: a target that already exists is never touched.
func MigrateLegacy() error {
	return errors.Join(migrateConfig(), migrateCache())
}

func migrateConfig() error {
	oldDir, err := legacyConfigDir()
	if err != nil {
		return err
	}
	newPath, err := ConfigPath("")
	if err != nil {
		return err
	}
	newDir := filepath.Dir(newPath)

	oldInfo, err := os.Stat(oldDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("config: checking %s: %w", oldDir, err)
	}
	if !oldInfo.IsDir() {
		return nil
	}
	if _, err := os.Stat(newDir); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("config: checking %s: %w", newDir, err)
	}

	staging := newDir + ".migrating"
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("config: clearing %s: %w", staging, err)
	}
	if err := copyTree(oldDir, staging); err != nil {
		return fmt.Errorf("config: copying %s: %w", oldDir, err)
	}
	if err := os.Rename(staging, newDir); err != nil {
		return fmt.Errorf("config: moving %s to %s: %w", staging, newDir, err)
	}
	return nil
}

func legacyConfigDir() (string, error) {
	if runtime.GOOS == "windows" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("could not determine config directory: %w", err)
		}
		return filepath.Join(dir, "vibez"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "vibez"), nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src) //nolint:gosec // path comes from our own config dir
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm) //nolint:gosec // path comes from our own config dir
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, perm)
}

func migrateCache() error {
	root, err := os.UserCacheDir()
	if err != nil {
		return fmt.Errorf("cache: determining cache directory: %w", err)
	}
	oldDir := filepath.Join(root, "vibez")
	newDir := filepath.Join(root, "aux")

	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("cache: checking %s: %w", oldDir, err)
	}
	if _, err := os.Stat(newDir); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("cache: checking %s: %w", newDir, err)
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		return fmt.Errorf("cache: moving %s to %s: %w", oldDir, newDir, err)
	}
	return nil
}
