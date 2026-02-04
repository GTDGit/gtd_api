package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/config"
)

// S3Service handles S3 operations using AWS Signature V4
type S3Service struct {
	bucket          string
	region          string
	endpoint        string
	accessKeyID     string
	secretAccessKey string
}

// NewS3Service creates a new S3 service
func NewS3Service(cfg *config.S3Config) (*S3Service, error) {
	if cfg == nil {
		return nil, fmt.Errorf("S3 config is nil")
	}

	return &S3Service{
		bucket:          cfg.Bucket,
		region:          cfg.Region,
		endpoint:        cfg.Endpoint,
		accessKeyID:     cfg.AccessKeyID,
		secretAccessKey: cfg.SecretAccessKey,
	}, nil
}

// UploadKTPDocument uploads KTP document to S3
func (s *S3Service) UploadKTPDocument(ctx context.Context, nik string, imageData []byte) (string, error) {
	key := fmt.Sprintf("identity/ktp/%s/document.jpg", nik)
	return s.uploadFile(ctx, key, imageData, "image/jpeg")
}

// UploadNPWPDocument uploads NPWP document to S3
func (s *S3Service) UploadNPWPDocument(ctx context.Context, npwpRaw string, imageData []byte) (string, error) {
	key := fmt.Sprintf("identity/npwp/%s/document.jpg", npwpRaw)
	return s.uploadFile(ctx, key, imageData, "image/jpeg")
}

// UploadSIMDocument uploads SIM document to S3
func (s *S3Service) UploadSIMDocument(ctx context.Context, simNumber string, imageData []byte) (string, error) {
	key := fmt.Sprintf("identity/sim/%s/document.jpg", simNumber)
	return s.uploadFile(ctx, key, imageData, "image/jpeg")
}

// uploadFile uploads a file to S3 using AWS Signature V4
func (s *S3Service) uploadFile(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	// Check if credentials are configured
	if s.accessKeyID == "" || s.secretAccessKey == "" {
		log.Warn().Str("key", key).Msg("S3 credentials not configured - skipping upload")
		return s.GetObjectURL(key), nil
	}

	// Build request URL
	url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, key)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	payloadHash := sha256Hex(data)

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))
	req.Header.Set("Host", fmt.Sprintf("%s.s3.%s.amazonaws.com", s.bucket, s.region))
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	// Sign request with AWS Signature V4
	authorization := s.signRequest(req, payloadHash, amzDate, dateStamp)
	req.Header.Set("Authorization", authorization)

	// Execute request
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Str("key", key).Msg("Failed to upload to S3")
		return "", fmt.Errorf("failed to upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		log.Error().
			Str("key", key).
			Int("status", resp.StatusCode).
			Str("response", string(body)).
			Msg("S3 upload failed")
		return "", fmt.Errorf("S3 upload failed: %s", string(body))
	}

	log.Info().Str("key", key).Msg("Successfully uploaded to S3")
	return s.GetObjectURL(key), nil
}

// signRequest creates AWS Signature V4 authorization header
func (s *S3Service) signRequest(req *http.Request, payloadHash, amzDate, dateStamp string) string {
	service := "s3"

	// Canonical request
	canonicalURI := req.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQueryString := ""

	// Canonical headers
	signedHeaders := []string{"content-type", "host", "x-amz-content-sha256", "x-amz-date"}
	sort.Strings(signedHeaders)

	var canonicalHeaders strings.Builder
	for _, h := range signedHeaders {
		canonicalHeaders.WriteString(strings.ToLower(h))
		canonicalHeaders.WriteString(":")
		canonicalHeaders.WriteString(strings.TrimSpace(req.Header.Get(h)))
		canonicalHeaders.WriteString("\n")
	}

	signedHeadersStr := strings.Join(signedHeaders, ";")

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		req.Method,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders.String(),
		signedHeadersStr,
		payloadHash,
	)

	// String to sign
	algorithm := "AWS4-HMAC-SHA256"
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, s.region, service)
	stringToSign := fmt.Sprintf("%s\n%s\n%s\n%s",
		algorithm,
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	)

	// Signing key
	kDate := hmacSHA256([]byte("AWS4"+s.secretAccessKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(s.region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))

	// Signature
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	// Authorization header
	return fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm,
		s.accessKeyID,
		credentialScope,
		signedHeadersStr,
		signature,
	)
}

// GetObjectURL returns the URL for an S3 object
func (s *S3Service) GetObjectURL(key string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, key)
}

// sha256Hex computes SHA256 hash and returns hex string
func sha256Hex(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// hmacSHA256 computes HMAC-SHA256
func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
