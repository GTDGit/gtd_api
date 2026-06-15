// Package storage provides private object storage for QRIS merchant documents
// and rendered Nobu batch files.
//
// Files are ALWAYS private: they are never served by a public URL. Documents are
// read back only by streaming through a token-validating handler (the files-qris
// portal); batch files are downloaded by an authenticated admin. The interface
// lets the byte backend be swapped — a local-disk driver is used for development
// today, and an S3 driver (private bucket, ap-southeast-3) is wired in once the
// real bucket is provisioned, without touching any caller.
package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GTDGit/gtd_api/internal/config"
)

// ErrNotFound is returned by Get when no object exists at the key.
var ErrNotFound = errors.New("storage: object not found")

// New builds a Storage from config. Driver "local" (default) writes to disk;
// "s3" is reserved for the private-bucket backend wired in once the real bucket
// is provisioned. An unknown driver is a configuration error.
func New(cfg config.StorageConfig) (Storage, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Driver)) {
	case "", "local":
		return NewLocalStorage(cfg.LocalBasePath)
	case "s3":
		return nil, fmt.Errorf("storage: s3 driver not yet implemented; set STORAGE_DRIVER=local")
	default:
		return nil, fmt.Errorf("storage: unknown driver %q", cfg.Driver)
	}
}

// Storage is the minimal object-storage contract used by the QRIS feature.
type Storage interface {
	// Put stores data under key with the given content type.
	Put(ctx context.Context, key, contentType string, data []byte) error
	// Get retrieves the bytes and content type stored under key. Returns
	// ErrNotFound when the key does not exist.
	Get(ctx context.Context, key string) (data []byte, contentType string, err error)
	// Delete removes the object at key. Deleting a missing key is not an error.
	Delete(ctx context.Context, key string) error
	// Driver returns the backend identifier ("local" | "s3") for logging.
	Driver() string
}

// localStorage is a development stub that writes objects to the local filesystem
// rooted at basePath. Content type is persisted in a sidecar file (<path>.ct) so
// Get can return it. This is NOT for production — it exists so the whole QRIS
// flow runs end-to-end before the real S3 bucket is provided.
type localStorage struct {
	basePath string
}

// NewLocalStorage builds a filesystem-backed Storage under basePath, creating
// the root directory if needed.
func NewLocalStorage(basePath string) (Storage, error) {
	basePath = strings.TrimSpace(basePath)
	if basePath == "" {
		basePath = "./data/qris-storage"
	}
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("storage: create base path: %w", err)
	}
	return &localStorage{basePath: basePath}, nil
}

func (s *localStorage) Driver() string { return "local" }

// resolve maps a storage key to an absolute filesystem path, guarding against
// path traversal (a key must stay within basePath).
func (s *localStorage) resolve(key string) (string, error) {
	clean := filepath.Clean("/" + strings.TrimSpace(key)) // force-rooted, strips ../
	full := filepath.Join(s.basePath, filepath.FromSlash(clean))
	absBase, err := filepath.Abs(s.basePath)
	if err != nil {
		return "", err
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if absFull != absBase && !strings.HasPrefix(absFull, absBase+string(os.PathSeparator)) {
		return "", fmt.Errorf("storage: key escapes base path: %q", key)
	}
	return absFull, nil
}

func (s *localStorage) Put(_ context.Context, key, contentType string, data []byte) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("storage: mkdir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("storage: write: %w", err)
	}
	if strings.TrimSpace(contentType) != "" {
		_ = os.WriteFile(path+".ct", []byte(contentType), 0o600)
	}
	return nil
}

func (s *localStorage) Get(_ context.Context, key string) ([]byte, string, error) {
	path, err := s.resolve(key)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", ErrNotFound
		}
		return nil, "", fmt.Errorf("storage: read: %w", err)
	}
	contentType := "application/octet-stream"
	if ct, ctErr := os.ReadFile(path + ".ct"); ctErr == nil && len(ct) > 0 {
		contentType = strings.TrimSpace(string(ct))
	}
	return data, contentType, nil
}

func (s *localStorage) Delete(_ context.Context, key string) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("storage: delete: %w", err)
	}
	_ = os.Remove(path + ".ct")
	return nil
}
