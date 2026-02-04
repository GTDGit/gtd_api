package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/config"
	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

// FaceCompareService handles face comparison operations
type FaceCompareService struct {
	repo  *repository.FaceCompareRepository
	s3Svc *S3Service
	cfg   *config.Config
}

// NewFaceCompareService creates a new face compare service
func NewFaceCompareService(
	repo *repository.FaceCompareRepository,
	s3Svc *S3Service,
	cfg *config.Config,
) *FaceCompareService {
	return &FaceCompareService{
		repo:  repo,
		s3Svc: s3Svc,
		cfg:   cfg,
	}
}

// FaceCompareError represents a face compare error with details
type FaceCompareError struct {
	Code    string
	Field   string
	Message string
}

func (e *FaceCompareError) Error() string {
	return e.Message
}

// CompareFacesResult represents the internal result of face comparison
type CompareFacesResult struct {
	Matched          bool
	Similarity       float64
	SourceFace       *FaceInfo
	TargetFace       *FaceInfo
	UnmatchedFaces   []FaceInfo
	ProcessingTimeMs int64
}

// FaceInfo represents detected face information
type FaceInfo struct {
	Confidence  float64
	BoundingBox *models.BoundingBox
}

// CompareFaces compares two faces using AWS Rekognition
func (s *FaceCompareService) CompareFaces(
	ctx context.Context,
	clientID int,
	sourceData []byte,
	targetData []byte,
	sourceType string,
	targetType string,
	sourceURL string,
	targetURL string,
	threshold *float64,
) (*models.FaceCompareResponse, error) {
	startTime := time.Now()

	// Call AWS Rekognition CompareFaces
	result, err := s.awsCompareFaces(ctx, sourceData, targetData, threshold)
	if err != nil {
		// Save error to database
		fc := &models.FaceCompare{
			ClientID:         clientID,
			SourceType:       sourceType,
			SourceURL:        sourceURL,
			TargetType:       targetType,
			TargetURL:        targetURL,
			Matched:          false,
			Similarity:       0,
			Threshold:        threshold,
			SourceDetected:   false,
			TargetDetected:   false,
			ProcessingTimeMs: time.Since(startTime).Milliseconds(),
			ErrorCode:        models.FaceCompareErrAWSServiceError,
			ErrorMessage:     err.Error(),
		}
		s.repo.Create(ctx, fc)
		return nil, err
	}

	// Build face compare record
	fc := &models.FaceCompare{
		ClientID:         clientID,
		SourceType:       sourceType,
		SourceURL:        sourceURL,
		TargetType:       targetType,
		TargetURL:        targetURL,
		Matched:          result.Matched,
		Similarity:       result.Similarity,
		Threshold:        threshold,
		SourceDetected:   result.SourceFace != nil,
		TargetDetected:   result.TargetFace != nil,
		ProcessingTimeMs: time.Since(startTime).Milliseconds(),
	}

	if result.SourceFace != nil {
		fc.SourceConfidence = &result.SourceFace.Confidence
		fc.SourceBoundingBox = result.SourceFace.BoundingBox
	}

	if result.TargetFace != nil {
		fc.TargetConfidence = &result.TargetFace.Confidence
		fc.TargetBoundingBox = result.TargetFace.BoundingBox
	}

	// Save to database
	if err := s.repo.Create(ctx, fc); err != nil {
		log.Error().Err(err).Msg("Failed to save face comparison result")
	}

	// Build response
	response := &models.FaceCompareResponse{
		ID:         fc.ID,
		Matched:    fc.Matched,
		Similarity: fc.Similarity,
		Threshold:  fc.Threshold,
		Source: models.FaceDetailResp{
			Detected:    fc.SourceDetected,
			Confidence:  getFloatValue(fc.SourceConfidence),
			BoundingBox: fc.SourceBoundingBox,
		},
		Target: models.FaceDetailResp{
			Detected:    fc.TargetDetected,
			Confidence:  getFloatValue(fc.TargetConfidence),
			BoundingBox: fc.TargetBoundingBox,
		},
	}

	return response, nil
}

// GetByID retrieves a face comparison by ID
func (s *FaceCompareService) GetByID(ctx context.Context, id string, clientID int) (*models.FaceCompare, error) {
	return s.repo.GetByIDAndClientID(ctx, id, clientID)
}

// awsCompareFaces calls AWS Rekognition CompareFaces API
func (s *FaceCompareService) awsCompareFaces(
	ctx context.Context,
	sourceImage []byte,
	targetImage []byte,
	threshold *float64,
) (*CompareFacesResult, error) {
	region := s.cfg.AWS.RekognitionRegion // ap-southeast-1 (Singapore)
	service := "rekognition"
	host := fmt.Sprintf("rekognition.%s.amazonaws.com", region)
	endpoint := fmt.Sprintf("https://%s", host)

	// Build request body
	reqBody := map[string]interface{}{
		"SourceImage": map[string]interface{}{
			"Bytes": base64.StdEncoding.EncodeToString(sourceImage),
		},
		"TargetImage": map[string]interface{}{
			"Bytes": base64.StdEncoding.EncodeToString(targetImage),
		},
	}

	// Add similarity threshold if provided
	if threshold != nil {
		reqBody["SimilarityThreshold"] = *threshold
	}

	bodyBytes, _ := json.Marshal(reqBody)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// AWS Signature V4
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	req.Header.Set("Host", host)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Target", "RekognitionService.CompareFaces")

	// Sign request
	authorization := s.signAWSRequest(req, bodyBytes, region, service, amzDate, dateStamp)
	req.Header.Set("Authorization", authorization)

	// Execute request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AWS request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	log.Debug().Str("response", string(body)).Msg("AWS CompareFaces Response")

	if resp.StatusCode != http.StatusOK {
		// Parse AWS error
		var awsErr struct {
			Type    string `json:"__type"`
			Message string `json:"message"`
		}
		json.Unmarshal(body, &awsErr)

		// Map AWS error to our error codes
		errCode := models.FaceCompareErrAWSServiceError
		if strings.Contains(awsErr.Type, "InvalidParameterException") {
			if strings.Contains(awsErr.Message, "no faces") || strings.Contains(awsErr.Message, "No face") {
				errCode = models.FaceCompareErrFaceNotDetected
			} else if strings.Contains(awsErr.Message, "multiple faces") {
				errCode = models.FaceCompareErrMultipleFaces
			} else if strings.Contains(awsErr.Message, "image") {
				errCode = models.FaceCompareErrInvalidImage
			}
		} else if strings.Contains(awsErr.Type, "ImageTooSmallException") {
			errCode = models.FaceCompareErrImageTooSmall
		}

		return nil, &FaceCompareError{
			Code:    errCode,
			Message: awsErr.Message,
		}
	}

	// Parse response
	var result struct {
		SourceImageFace *struct {
			Confidence  float64 `json:"Confidence"`
			BoundingBox struct {
				Width  float64 `json:"Width"`
				Height float64 `json:"Height"`
				Left   float64 `json:"Left"`
				Top    float64 `json:"Top"`
			} `json:"BoundingBox"`
		} `json:"SourceImageFace"`
		FaceMatches []struct {
			Similarity float64 `json:"Similarity"`
			Face       struct {
				Confidence  float64 `json:"Confidence"`
				BoundingBox struct {
					Width  float64 `json:"Width"`
					Height float64 `json:"Height"`
					Left   float64 `json:"Left"`
					Top    float64 `json:"Top"`
				} `json:"BoundingBox"`
			} `json:"Face"`
		} `json:"FaceMatches"`
		UnmatchedFaces []struct {
			Confidence  float64 `json:"Confidence"`
			BoundingBox struct {
				Width  float64 `json:"Width"`
				Height float64 `json:"Height"`
				Left   float64 `json:"Left"`
				Top    float64 `json:"Top"`
			} `json:"BoundingBox"`
		} `json:"UnmatchedFaces"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse AWS response: %w", err)
	}

	compareResult := &CompareFacesResult{
		Matched:    false,
		Similarity: 0,
	}

	// Source face info
	if result.SourceImageFace != nil {
		compareResult.SourceFace = &FaceInfo{
			Confidence: result.SourceImageFace.Confidence,
			BoundingBox: &models.BoundingBox{
				Width:  result.SourceImageFace.BoundingBox.Width,
				Height: result.SourceImageFace.BoundingBox.Height,
				Left:   result.SourceImageFace.BoundingBox.Left,
				Top:    result.SourceImageFace.BoundingBox.Top,
			},
		}
	}

	// Check if faces match
	if len(result.FaceMatches) > 0 {
		match := result.FaceMatches[0]
		compareResult.Matched = true
		compareResult.Similarity = match.Similarity
		compareResult.TargetFace = &FaceInfo{
			Confidence: match.Face.Confidence,
			BoundingBox: &models.BoundingBox{
				Width:  match.Face.BoundingBox.Width,
				Height: match.Face.BoundingBox.Height,
				Left:   match.Face.BoundingBox.Left,
				Top:    match.Face.BoundingBox.Top,
			},
		}
	} else if len(result.UnmatchedFaces) > 0 {
		// No match but faces detected in target
		unmatch := result.UnmatchedFaces[0]
		compareResult.TargetFace = &FaceInfo{
			Confidence: unmatch.Confidence,
			BoundingBox: &models.BoundingBox{
				Width:  unmatch.BoundingBox.Width,
				Height: unmatch.BoundingBox.Height,
				Left:   unmatch.BoundingBox.Left,
				Top:    unmatch.BoundingBox.Top,
			},
		}
	}

	return compareResult, nil
}

// signAWSRequest signs an AWS request using Signature V4
func (s *FaceCompareService) signAWSRequest(req *http.Request, body []byte, region, service, amzDate, dateStamp string) string {
	// Canonical request
	payloadHash := sha256HexFC(body)

	canonicalURI := "/"
	canonicalQueryString := ""

	signedHeaders := []string{"content-type", "host", "x-amz-date", "x-amz-target"}
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
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)
	stringToSign := fmt.Sprintf("%s\n%s\n%s\n%s",
		algorithm,
		amzDate,
		credentialScope,
		sha256HexFC([]byte(canonicalRequest)),
	)

	// Signing key
	kDate := hmacSHA256FC([]byte("AWS4"+s.cfg.AWS.SecretAccessKey), []byte(dateStamp))
	kRegion := hmacSHA256FC(kDate, []byte(region))
	kService := hmacSHA256FC(kRegion, []byte(service))
	kSigning := hmacSHA256FC(kService, []byte("aws4_request"))

	// Signature
	signature := hex.EncodeToString(hmacSHA256FC(kSigning, []byte(stringToSign)))

	// Authorization header
	return fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm,
		s.cfg.AWS.AccessKeyID,
		credentialScope,
		signedHeadersStr,
		signature,
	)
}

// DownloadFromS3URL downloads image from S3 URL
func (s *FaceCompareService) DownloadFromS3URL(ctx context.Context, url string) ([]byte, error) {
	// Simple HTTP GET for public S3 URLs or signed URLs
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &FaceCompareError{
			Code:    models.FaceCompareErrS3AccessError,
			Message: fmt.Sprintf("Failed to access S3 URL: status %d", resp.StatusCode),
		}
	}

	return io.ReadAll(resp.Body)
}

// Helper functions
func getFloatValue(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

// sha256HexFC computes SHA256 hash and returns hex string
func sha256HexFC(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// hmacSHA256FC computes HMAC-SHA256
func hmacSHA256FC(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
