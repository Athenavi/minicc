package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Store implements FileStore backed by an S3-compatible object store (MinIO / AWS S3).
type S3Store struct {
	client *minio.Client
	bucket string
	prefix string // optional subdirectory prefix within the bucket
}

// NewS3Store creates a new S3Store connected to the given endpoint.
// endpoint: S3-compatible endpoint (e.g. "play.min.io" or "s3.amazonaws.com").
// bucket:   the bucket name (created if it doesn't exist).
// prefix:   optional prefix (subdirectory) — all paths are relative to this.
// creds:    (accessKey, secretKey, token) — token may be empty for static creds.
func NewS3Store(endpoint, bucket, prefix, accessKey, secretKey, token string, useSSL bool) (*S3Store, error) {
	if bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, token),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Ensure bucket exists (idempotent)
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		// Check if bucket already exists
		exists, errExists := client.BucketExists(ctx, bucket)
		if errExists != nil || !exists {
			return nil, fmt.Errorf("ensure bucket %q: %w", bucket, err)
		}
	}

	return &S3Store{
		client: client,
		bucket: bucket,
		prefix: prefix,
	}, nil
}

func (s *S3Store) Read(ctx context.Context, path string) ([]byte, error) {
	objPath := s.objectPath(path)

	obj, err := s.client.GetObject(ctx, s.bucket, objPath, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("s3 get object %q: %w", objPath, err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("s3 read object %q: %w", objPath, err)
	}

	return data, nil
}

func (s *S3Store) Write(ctx context.Context, path string, data []byte) error {
	objPath := s.objectPath(path)

	r := bytes.NewReader(data)
	_, err := s.client.PutObject(ctx, s.bucket, objPath, r, int64(len(data)), minio.PutObjectOptions{
		ContentType: detectContentType(path),
	})
	if err != nil {
		return fmt.Errorf("s3 put object %q: %w", objPath, err)
	}

	return nil
}

func (s *S3Store) Delete(ctx context.Context, path string) error {
	objPath := s.objectPath(path)

	err := s.client.RemoveObject(ctx, s.bucket, objPath, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("s3 remove object %q: %w", objPath, err)
	}

	return nil
}

func (s *S3Store) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	objPrefix := s.objectPath(prefix)

	var files []FileInfo
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    objPrefix,
		Recursive: false,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("s3 list objects: %w", obj.Err)
		}
		files = append(files, FileInfo{
			Path:     s.stripPrefix(obj.Key),
			Size:     obj.Size,
			IsDir:    obj.Key[len(obj.Key)-1] == '/',
			Modified: obj.LastModified.Format("2006-01-02T15:04:05Z"),
		})
	}

	if files == nil {
		files = []FileInfo{}
	}
	return files, nil
}

// objectPath joins the store prefix with the user-provided path.
func (s *S3Store) objectPath(path string) string {
	if s.prefix == "" {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(s.prefix + "/" + path)
}

// stripPrefix removes the store prefix from an S3 object key.
func (s *S3Store) stripPrefix(key string) string {
	if s.prefix == "" || len(key) <= len(s.prefix)+1 {
		return key
	}
	return key[len(s.prefix)+1:]
}

func detectContentType(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".md":
		return "text/markdown"
	case ".txt":
		return "text/plain"
	case ".yaml", ".yml":
		return "application/x-yaml"
	case ".zip":
		return "application/zip"
	case ".gz", ".tar.gz":
		return "application/gzip"
	default:
		return "application/octet-stream"
	}
}
