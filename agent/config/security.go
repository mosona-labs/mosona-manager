package config

import (
	"errors"
	"fmt"
	"io"
	"mosona-manager/agent/runtime"
	"os"
	"path/filepath"
)

func secureInstallDir() error {
	info, err := os.Lstat(runtime.InstallDir)
	if err != nil {
		return fmt.Errorf("inspect agent install directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("agent install directory must be a real directory")
	}
	if err := validateOwner(runtime.InstallDir, info); err != nil {
		return err
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(runtime.InstallDir, 0o700); err != nil {
			return fmt.Errorf("secure agent install directory: %w", err)
		}
	}
	return nil
}

func readSecureFile(path string, mode os.FileMode) ([]byte, error) {
	if err := secureInstallDir(); err != nil {
		return nil, err
	}
	file, err := openFileNoFollow(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	if err := secureRegularFile(path, file, mode); err != nil {
		return nil, err
	}
	return io.ReadAll(file)
}

func writeSecureFile(path string, data []byte, mode os.FileMode) error {
	if err := secureInstallDir(); err != nil {
		return err
	}
	file, err := openFileNoFollow(path, os.O_CREATE|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if err := secureRegularFile(path, file, mode); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Truncate(0); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func createSecureFile(path string, data []byte, mode os.FileMode) (err error) {
	if err := secureInstallDir(); err != nil {
		return err
	}
	file, err := openFileNoFollow(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	created := true
	defer func() {
		_ = file.Close()
		if err != nil && created {
			_ = os.Remove(path)
		}
	}()
	if err = secureRegularFile(path, file, mode); err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	created = false
	return nil
}

func secureRegularFile(path string, file *os.File, mode os.FileMode) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect %s: %w", filepath.Base(path), err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", filepath.Base(path))
	}
	if err := validateOwner(path, info); err != nil {
		return err
	}
	if info.Mode().Perm() != mode {
		if err := file.Chmod(mode); err != nil {
			return fmt.Errorf("secure %s permissions: %w", filepath.Base(path), err)
		}
	}
	return nil
}
