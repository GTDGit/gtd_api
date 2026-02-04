package models

import "time"

// FaceCompare represents a face comparison record
type FaceCompare struct {
	ID                string       `json:"id" db:"id"`
	ClientID          int          `json:"clientId" db:"client_id"`
	SourceType        string       `json:"sourceType" db:"source_type"` // 'url' or 'upload'
	SourceURL         string       `json:"sourceUrl" db:"source_url"`
	TargetType        string       `json:"targetType" db:"target_type"` // 'url' or 'upload'
	TargetURL         string       `json:"targetUrl" db:"target_url"`
	Matched           bool         `json:"matched" db:"matched"`
	Similarity        float64      `json:"similarity" db:"similarity"`
	Threshold         *float64     `json:"threshold,omitempty" db:"threshold"` // Optional threshold
	SourceDetected    bool         `json:"sourceDetected" db:"source_detected"`
	SourceConfidence  *float64     `json:"sourceConfidence,omitempty" db:"source_confidence"`
	SourceBoundingBox *BoundingBox `json:"sourceBoundingBox,omitempty" db:"-"`
	TargetDetected    bool         `json:"targetDetected" db:"target_detected"`
	TargetConfidence  *float64     `json:"targetConfidence,omitempty" db:"target_confidence"`
	TargetBoundingBox *BoundingBox `json:"targetBoundingBox,omitempty" db:"-"`
	ProcessingTimeMs  int64        `json:"processingTimeMs,omitempty" db:"processing_time_ms"`
	AWSRequestID      string       `json:"awsRequestId,omitempty" db:"aws_request_id"`
	ErrorCode         string       `json:"errorCode,omitempty" db:"error_code"`
	ErrorMessage      string       `json:"errorMessage,omitempty" db:"error_message"`
	CreatedAt         time.Time    `json:"createdAt" db:"created_at"`
}

// BoundingBox represents face bounding box coordinates
type BoundingBox struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
}

// FaceCompareRequest represents the JSON request for face comparison
type FaceCompareRequest struct {
	Source              string   `json:"source" binding:"required"`
	Target              string   `json:"target" binding:"required"`
	SimilarityThreshold *float64 `json:"similarityThreshold,omitempty"`
}

// FaceCompareResponse represents the response for face comparison
type FaceCompareResponse struct {
	ID         string         `json:"id"`
	Matched    bool           `json:"matched"`
	Similarity float64        `json:"similarity"`
	Threshold  *float64       `json:"threshold"`
	Source     FaceDetailResp `json:"source"`
	Target     FaceDetailResp `json:"target"`
}

// FaceDetailResp represents face detection details in response
type FaceDetailResp struct {
	Detected    bool         `json:"detected"`
	Confidence  float64      `json:"confidence,omitempty"`
	BoundingBox *BoundingBox `json:"boundingBox,omitempty"`
}

// FaceCompareError codes
const (
	FaceCompareErrFaceNotDetected = "FACE_NOT_DETECTED"
	FaceCompareErrMultipleFaces   = "MULTIPLE_FACES"
	FaceCompareErrInvalidImage    = "INVALID_IMAGE"
	FaceCompareErrImageTooSmall   = "IMAGE_TOO_SMALL"
	FaceCompareErrS3AccessError   = "S3_ACCESS_ERROR"
	FaceCompareErrAWSServiceError = "AWS_SERVICE_ERROR"
)
