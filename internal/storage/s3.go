package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/GTDGit/gtd_api/internal/config"
)

// s3Storage stores QRIS documents and batch files in a private S3 bucket. Keys
// arrive already namespaced by the service layer (it prepends KeyPrefix), so the
// driver uses them verbatim. Objects are uploaded with no ACL — they inherit the
// bucket's public-access-block policy and are never served by a public URL.
type s3Storage struct {
	client *s3.Client
	bucket string
}

// NewS3Storage builds an S3-backed Storage from config. Region and bucket are
// required; static credentials are used when provided, otherwise the default
// AWS credential chain. A custom Endpoint (MinIO) switches to path-style.
func NewS3Storage(cfg config.StorageConfig) (Storage, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("storage: S3_BUCKET is required for s3 driver")
	}
	ctx := context.Background()

	var opts []func(*awsconfig.LoadOptions) error
	if r := strings.TrimSpace(cfg.Region); r != "" {
		opts = append(opts, awsconfig.WithRegion(r))
	}
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("storage: load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if ep := strings.TrimSpace(cfg.Endpoint); ep != "" {
			o.BaseEndpoint = aws.String(ep)
			o.UsePathStyle = true
		}
	})

	return &s3Storage{client: client, bucket: cfg.Bucket}, nil
}

func (s *s3Storage) Driver() string { return "s3" }

func (s *s3Storage) Put(ctx context.Context, key, contentType string, data []byte) error {
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("storage: put %s: %w", key, err)
	}
	return nil
}

func (s *s3Storage) Get(ctx context.Context, key string) ([]byte, string, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isS3NotFound(err) {
			return nil, "", ErrNotFound
		}
		return nil, "", fmt.Errorf("storage: get %s: %w", key, err)
	}
	defer out.Body.Close()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(out.Body); err != nil {
		return nil, "", fmt.Errorf("storage: read %s: %w", key, err)
	}
	ct := "application/octet-stream"
	if out.ContentType != nil && *out.ContentType != "" {
		ct = *out.ContentType
	}
	return buf.Bytes(), ct, nil
}

func (s *s3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil && !isS3NotFound(err) {
		return fmt.Errorf("storage: delete %s: %w", key, err)
	}
	return nil
}

// isS3NotFound reports whether err is a missing-key error (NoSuchKey / 404),
// so Get can return the storage.ErrNotFound sentinel.
func isS3NotFound(err error) bool {
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nf *types.NotFound
	if errors.As(err, &nf) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}
