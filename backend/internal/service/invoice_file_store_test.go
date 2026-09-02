package service

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func newInvoiceTestStore(t *testing.T) *InvoiceFileStore {
	t.Helper()
	return NewInvoiceFileStore(&config.Config{Invoice: config.InvoiceStorageConfig{
		StorageDir: t.TempDir(), StorageMinFreeMB: 128,
	}})
}

func TestInvoiceFileStoreDefaultsDoNotTouchDiskUntilChecked(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	store := NewInvoiceFileStore(&config.Config{Invoice: config.InvoiceStorageConfig{StorageDir: root, StorageMinFreeMB: 128}})
	require.NotNil(t, store)
	_, err := os.Stat(root)
	require.True(t, os.IsNotExist(err))
}

func TestInvoiceFileStorePreparesAndReadsPDF(t *testing.T) {
	store := newInvoiceTestStore(t)
	prepared, err := store.Prepare(context.Background(), 42, 7, InvoiceUploadInput{
		Filename: "invoice.pdf", Reader: bytes.NewReader([]byte("%PDF-1.7\n%%EOF")),
	}, 1024)
	require.NoError(t, err)
	require.Equal(t, "application/pdf", prepared.Metadata.ContentType)
	require.NotContains(t, prepared.Metadata.StorageKey, "invoice.pdf")

	reader, err := store.Open(prepared.Metadata.StorageKey)
	require.NoError(t, err)
	defer reader.Close()
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "%PDF-1.7\n%%EOF", string(data))
}

func TestInvoiceFileStoreVerifiesSizeAndSHA256BeforeDownload(t *testing.T) {
	store := newInvoiceTestStore(t)
	prepared, err := store.Prepare(context.Background(), 42, 7, InvoiceUploadInput{
		Filename: "invoice.pdf", Reader: bytes.NewReader([]byte("%PDF-1.7\n%%EOF")),
	}, 1024)
	require.NoError(t, err)

	reader, err := store.OpenVerified(prepared.Metadata)
	require.NoError(t, err)
	require.NoError(t, reader.Close())

	path, err := store.resolveStorageKey(prepared.Metadata.StorageKey, true)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("%PDF-1.7\n%%EOX"), 0o600))
	_, err = store.OpenVerified(prepared.Metadata)
	require.Equal(t, "INVOICE_FILE_INTEGRITY_FAILED", infraerrors.Reason(err))

	invalidMetadata := prepared.Metadata
	invalidMetadata.SHA256 = "not-a-sha256"
	_, err = store.OpenVerified(invalidMetadata)
	require.Equal(t, "INVOICE_FILE_INTEGRITY_FAILED", infraerrors.Reason(err))
}

func TestInvoiceFileStoreRejectsInvalidTypeSizeAndTraversal(t *testing.T) {
	store := newInvoiceTestStore(t)
	_, err := store.Prepare(context.Background(), 42, 7, InvoiceUploadInput{Filename: "invoice.pdf", Reader: bytes.NewReader([]byte("not-pdf"))}, 1024)
	require.Equal(t, "INVOICE_FILE_INVALID", infraerrors.Reason(err))
	_, err = store.Prepare(context.Background(), 42, 7, InvoiceUploadInput{Filename: "invoice.pdf", Reader: bytes.NewReader([]byte("%PDF-1.7\nmissing trailer"))}, 1024)
	require.Equal(t, "INVOICE_FILE_INVALID", infraerrors.Reason(err))
	_, err = store.Prepare(context.Background(), 42, 7, InvoiceUploadInput{Filename: "invoice.exe", Reader: bytes.NewReader([]byte("%PDF-1.7"))}, 1024)
	require.Equal(t, "INVOICE_FILE_TYPE_UNSUPPORTED", infraerrors.Reason(err))
	_, err = store.Prepare(context.Background(), 42, 7, InvoiceUploadInput{Filename: "invoice.pdf", Reader: bytes.NewReader([]byte("%PDF-123456"))}, 5)
	require.Equal(t, "INVOICE_FILE_TOO_LARGE", infraerrors.Reason(err))
	_, err = store.Open("../../outside.pdf")
	require.Equal(t, "INVOICE_FILE_NOT_FOUND", infraerrors.Reason(err))
}

func TestInvoiceFileStoreListsFilesWithoutTemporaryUploads(t *testing.T) {
	store := newInvoiceTestStore(t)
	prepared, err := store.Prepare(context.Background(), 42, 7, InvoiceUploadInput{
		Filename: "invoice.pdf", Reader: bytes.NewReader([]byte("%PDF-1.7\n%%EOF")),
	}, 1024)
	require.NoError(t, err)
	tempDir := filepath.Join(store.root, ".tmp")
	require.NoError(t, os.MkdirAll(tempDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "unfinished"), []byte("partial"), 0o600))
	orphan := filepath.Join(store.root, "2026", "08", "orphan.pdf")
	require.NoError(t, os.MkdirAll(filepath.Dir(orphan), 0o700))
	require.NoError(t, os.WriteFile(orphan, []byte("%PDF-1.7\n%%EOF"), 0o600))

	keys, err := store.ListStorageKeys(context.Background())
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"2026/08/orphan.pdf", prepared.Metadata.StorageKey}, keys)
}

func TestInvoiceFileStoreValidatesOFDContainer(t *testing.T) {
	var valid bytes.Buffer
	writer := zip.NewWriter(&valid)
	entry, err := writer.Create("OFD.xml")
	require.NoError(t, err)
	_, err = entry.Write([]byte(`<ofd:OFD xmlns:ofd="http://www.ofdspec.org/2016"/>`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	_, _, contentType, err := validateInvoiceFile("invoice.ofd", valid.Bytes())
	require.NoError(t, err)
	require.Equal(t, "application/ofd", contentType)

	var invalid bytes.Buffer
	writer = zip.NewWriter(&invalid)
	entry, err = writer.Create("Doc_0/Document.xml")
	require.NoError(t, err)
	_, err = entry.Write([]byte("document"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	_, _, _, err = validateInvoiceFile("invoice.ofd", invalid.Bytes())
	require.Equal(t, "INVOICE_FILE_INVALID", infraerrors.Reason(err))
}
