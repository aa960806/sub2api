package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	invoiceHardMaxFileBytes = int64(20 << 20)
	invoiceOFDMaxFiles      = 1000
	invoiceOFDMaxEntryBytes = int64(32 << 20)
	invoiceOFDMaxTotalBytes = int64(100 << 20)
)

type InvoiceFileStore struct {
	root         string
	minFreeBytes uint64
	configError  error
}

func NewInvoiceFileStore(cfg *config.Config) *InvoiceFileStore {
	root := "/app/data/invoices"
	minFreeMB := int64(512)
	var configError error
	if cfg != nil {
		if value := strings.TrimSpace(cfg.Invoice.StorageDir); value != "" {
			root = value
		}
		if cfg.Invoice.StorageMinFreeMB >= 128 {
			minFreeMB = cfg.Invoice.StorageMinFreeMB
		} else if cfg.Invoice.StorageMinFreeMB > 0 {
			configError = fmt.Errorf("invoice storage minimum free space must be at least 128 MiB")
		}
	}
	return &InvoiceFileStore{root: filepath.Clean(root), minFreeBytes: uint64(minFreeMB) << 20, configError: configError}
}

func (s *InvoiceFileStore) CheckReady(ctx context.Context) (InvoiceStorageStatus, error) {
	status := InvoiceStorageStatus{CheckedAt: time.Now()}
	if s != nil && s.configError != nil {
		status.FailureReason = "storage_config_invalid"
		return status, s.configError
	}
	if s == nil || strings.TrimSpace(s.root) == "" {
		status.FailureReason = "storage_not_configured"
		return status, fmt.Errorf("invoice storage is not configured")
	}
	if err := ctx.Err(); err != nil {
		return status, err
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		status.FailureReason = "directory_unavailable"
		return status, err
	}
	info, err := os.Lstat(s.root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		status.FailureReason = "directory_invalid"
		if err == nil {
			err = fmt.Errorf("invoice storage root must be a real directory")
		}
		return status, err
	}
	probe, err := os.CreateTemp(s.root, ".invoice-write-probe-")
	if err != nil {
		status.FailureReason = "directory_not_writable"
		return status, err
	}
	probeName := probe.Name()
	_ = probe.Chmod(0o600)
	closeErr := probe.Close()
	_ = os.Remove(probeName)
	if closeErr != nil {
		status.FailureReason = "directory_not_writable"
		return status, closeErr
	}
	free, err := invoiceDiskFreeBytes(s.root)
	if err != nil {
		status.FailureReason = "capacity_check_failed"
		return status, err
	}
	status.FreeBytes = free
	if free < s.minFreeBytes {
		status.FailureReason = "insufficient_free_space"
		return status, fmt.Errorf("invoice storage free space is below the configured reserve")
	}
	status.Available = true
	return status, nil
}

func (s *InvoiceFileStore) Prepare(ctx context.Context, requestID, adminID int64, input InvoiceUploadInput, maxBytes int64) (*InvoicePreparedFile, error) {
	if input.Reader == nil || requestID <= 0 || adminID <= 0 {
		return nil, infraerrors.BadRequest("INVOICE_FILE_INVALID", "invoice file is invalid")
	}
	if maxBytes <= 0 || maxBytes > invoiceHardMaxFileBytes {
		maxBytes = invoiceHardMaxFileBytes
	}
	status, err := s.CheckReady(ctx)
	if err != nil || status.FreeBytes < uint64(maxBytes)+s.minFreeBytes {
		return nil, infraerrors.Conflict("INVOICE_STORAGE_UNAVAILABLE", "invoice storage is unavailable").WithCause(err)
	}
	data, err := io.ReadAll(io.LimitReader(input.Reader, maxBytes+1))
	if err != nil {
		return nil, infraerrors.BadRequest("INVOICE_FILE_INVALID", "invoice file could not be read").WithCause(err)
	}
	if int64(len(data)) > maxBytes {
		return nil, infraerrors.BadRequest("INVOICE_FILE_TOO_LARGE", "invoice file exceeds the configured limit")
	}
	filename, ext, contentType, err := validateInvoiceFile(input.Filename, data)
	if err != nil {
		return nil, err
	}
	token := make([]byte, 24)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	storageKey := filepath.ToSlash(filepath.Join(now.Format("2006"), now.Format("01"), fmt.Sprintf("%d", requestID), hex.EncodeToString(token)+"."+ext))
	finalPath, err := s.resolveStorageKey(storageKey, false)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o700); err != nil {
		return nil, infraerrors.Conflict("INVOICE_STORAGE_UNAVAILABLE", "invoice storage is unavailable").WithCause(err)
	}
	tempDir := filepath.Join(s.root, ".tmp")
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		return nil, infraerrors.Conflict("INVOICE_STORAGE_UNAVAILABLE", "invoice storage is unavailable").WithCause(err)
	}
	temp, err := os.CreateTemp(tempDir, "upload-*")
	if err != nil {
		return nil, infraerrors.Conflict("INVOICE_STORAGE_UNAVAILABLE", "invoice storage is unavailable").WithCause(err)
	}
	tempName := temp.Name()
	keep := false
	defer func() {
		_ = temp.Close()
		if !keep {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return nil, err
	}
	if _, err := temp.Write(data); err != nil {
		return nil, err
	}
	if err := temp.Sync(); err != nil {
		return nil, err
	}
	if err := temp.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tempName, finalPath); err != nil {
		return nil, err
	}
	keep = true
	if dir, openErr := os.Open(filepath.Dir(finalPath)); openErr == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	digest := sha256.Sum256(data)
	return &InvoicePreparedFile{Metadata: InvoiceFileMetadata{
		InvoiceRequestID: requestID,
		StorageKey:       storageKey,
		OriginalFilename: filename,
		ContentType:      contentType,
		FileExtension:    ext,
		FileSize:         int64(len(data)),
		SHA256:           hex.EncodeToString(digest[:]),
		IsCurrent:        true,
		UploadedBy:       adminID,
		UploadedAt:       now,
	}}, nil
}

func (s *InvoiceFileStore) DeletePrepared(file *InvoicePreparedFile) error {
	if file == nil {
		return nil
	}
	path, err := s.resolveStorageKey(file.Metadata.StorageKey, true)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *InvoiceFileStore) Open(storageKey string) (io.ReadCloser, error) {
	path, err := s.resolveStorageKey(storageKey, true)
	if err != nil {
		return nil, infraerrors.NotFound("INVOICE_FILE_NOT_FOUND", "invoice file was not found")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, infraerrors.NotFound("INVOICE_FILE_NOT_FOUND", "invoice file was not found").WithCause(err)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, infraerrors.NotFound("INVOICE_FILE_NOT_FOUND", "invoice file was not found")
	}
	return file, nil
}

func (s *InvoiceFileStore) OpenVerified(metadata InvoiceFileMetadata) (io.ReadCloser, error) {
	if metadata.FileSize <= 0 || metadata.FileSize > invoiceHardMaxFileBytes {
		return nil, invoiceFileIntegrityError(fmt.Errorf("invalid stored invoice file size"))
	}
	expectedDigest, err := hex.DecodeString(strings.TrimSpace(metadata.SHA256))
	if err != nil || len(expectedDigest) != sha256.Size {
		return nil, invoiceFileIntegrityError(fmt.Errorf("invalid stored invoice file digest"))
	}

	reader, err := s.Open(metadata.StorageKey)
	if err != nil {
		return nil, err
	}
	closeWithError := func(cause error) (io.ReadCloser, error) {
		_ = reader.Close()
		return nil, invoiceFileIntegrityError(cause)
	}

	hasher := sha256.New()
	actualSize, err := io.Copy(hasher, io.LimitReader(reader, invoiceHardMaxFileBytes+1))
	if err != nil {
		return closeWithError(err)
	}
	if actualSize != metadata.FileSize || !bytes.Equal(hasher.Sum(nil), expectedDigest) {
		return closeWithError(fmt.Errorf("stored invoice file does not match its database metadata"))
	}
	seeker, ok := reader.(io.Seeker)
	if !ok {
		return closeWithError(fmt.Errorf("stored invoice file cannot be rewound"))
	}
	if _, err := seeker.Seek(0, io.SeekStart); err != nil {
		return closeWithError(err)
	}
	return reader, nil
}

func invoiceFileIntegrityError(cause error) error {
	return infraerrors.InternalServer("INVOICE_FILE_INTEGRITY_FAILED", "invoice file failed integrity verification").WithCause(cause)
}

func (s *InvoiceFileStore) ListStorageKeys(ctx context.Context) ([]string, error) {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return nil, infraerrors.Conflict("INVOICE_STORAGE_UNAVAILABLE", "invoice storage is unavailable")
	}
	rootInfo, err := os.Lstat(s.root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, infraerrors.Conflict("INVOICE_STORAGE_UNAVAILABLE", "invoice storage is unavailable").WithCause(err)
	}
	keys := make([]string, 0)
	err = filepath.WalkDir(s.root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == s.root {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() && rel == ".tmp" {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("invoice storage contains a symbolic link")
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("invoice storage contains a non-regular file")
		}
		keys = append(keys, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *InvoiceFileStore) resolveStorageKey(storageKey string, mustExist bool) (string, error) {
	if s == nil || filepath.IsAbs(storageKey) || strings.ContainsRune(storageKey, '\x00') {
		return "", fmt.Errorf("invalid invoice storage key")
	}
	clean := filepath.Clean(filepath.FromSlash(storageKey))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid invoice storage key")
	}
	rootAbs, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Join(rootAbs, clean))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invoice storage key escapes root")
	}
	if mustExist {
		rootReal, err := filepath.EvalSymlinks(rootAbs)
		if err != nil {
			return "", err
		}
		pathReal, err := filepath.EvalSymlinks(pathAbs)
		if err != nil {
			return "", err
		}
		realRel, err := filepath.Rel(rootReal, pathReal)
		if err != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("invoice file escapes storage root")
		}
	}
	return pathAbs, nil
}

func validateInvoiceFile(originalName string, data []byte) (string, string, string, error) {
	name := strings.TrimSpace(filepath.Base(strings.ReplaceAll(originalName, "\\", "/")))
	if name == "" || name == "." || len([]byte(name)) > 255 {
		return "", "", "", infraerrors.BadRequest("INVOICE_FILE_INVALID", "invoice filename is invalid")
	}
	for _, value := range name {
		if unicode.IsControl(value) {
			return "", "", "", infraerrors.BadRequest("INVOICE_FILE_INVALID", "invoice filename is invalid")
		}
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	switch ext {
	case "pdf":
		if len(data) < 8 || !bytes.Equal(data[:5], []byte("%PDF-")) || !bytes.Contains(data[maxInvoiceInt(0, len(data)-1024):], []byte("%%EOF")) {
			return "", "", "", infraerrors.BadRequest("INVOICE_FILE_INVALID", "invoice PDF signature is invalid")
		}
		return name, ext, "application/pdf", nil
	case "ofd":
		if err := validateOFD(data); err != nil {
			return "", "", "", err
		}
		return name, ext, "application/ofd", nil
	default:
		return "", "", "", infraerrors.BadRequest("INVOICE_FILE_TYPE_UNSUPPORTED", "only PDF and OFD invoice files are supported")
	}
}

func maxInvoiceInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func validateOFD(data []byte) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(reader.File) == 0 || len(reader.File) > invoiceOFDMaxFiles {
		return infraerrors.BadRequest("INVOICE_FILE_INVALID", "invoice OFD container is invalid")
	}
	foundRoot := false
	var total uint64
	for _, entry := range reader.File {
		name := filepath.ToSlash(entry.Name)
		clean := filepath.ToSlash(filepath.Clean(name))
		if strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || clean == ".." {
			return infraerrors.BadRequest("INVOICE_FILE_INVALID", "invoice OFD container contains an invalid path")
		}
		if strings.EqualFold(clean, "OFD.xml") {
			foundRoot = true
		}
		if entry.UncompressedSize64 > uint64(invoiceOFDMaxEntryBytes) {
			return infraerrors.BadRequest("INVOICE_FILE_INVALID", "invoice OFD entry is too large")
		}
		total += entry.UncompressedSize64
		if total > uint64(invoiceOFDMaxTotalBytes) {
			return infraerrors.BadRequest("INVOICE_FILE_INVALID", "invoice OFD expanded size is too large")
		}
	}
	if !foundRoot {
		return infraerrors.BadRequest("INVOICE_FILE_INVALID", "invoice OFD root document is missing")
	}
	return nil
}
