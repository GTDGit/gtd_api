package models

import "time"

// LivenessMethod represents the liveness detection method
type LivenessMethod string

const (
	LivenessMethodPassive  LivenessMethod = "passive"
	LivenessMethodActive   LivenessMethod = "active"
	LivenessMethodGaze     LivenessMethod = "gaze"
	LivenessMethodSpectrum LivenessMethod = "spectrum"
	LivenessMethodSecure   LivenessMethod = "secure"
)

// LivenessSession represents a liveness session
type LivenessSession struct {
	ID            string         `json:"id" db:"id"`
	ClientID      int            `json:"clientId" db:"client_id"`
	NIK           string         `json:"nik" db:"nik"`
	SessionID     string         `json:"sessionId" db:"session_id"`
	Method        LivenessMethod `json:"method" db:"method"`
	Status        string         `json:"status" db:"status"` // Pending, Processing, Passed, Failed, Expired
	IsLive        *bool          `json:"isLive,omitempty" db:"is_live"`
	Confidence    *float64       `json:"confidence,omitempty" db:"confidence"`
	FaceURL       string         `json:"faceUrl,omitempty" db:"face_url"`
	FailureReason string         `json:"failureReason,omitempty" db:"failure_reason"`
	// BizToken       string         `json:"bizToken" db:"biz_token"` -- Removed for AWS
	// LivenessURL    string         `json:"livenessUrl" db:"liveness_url"` -- Removed for AWS
	Challenges     []string    `json:"challenges,omitempty" db:"-"`     // For active method
	GazePoints     []GazePoint `json:"gazePoints,omitempty" db:"-"`     // For gaze method
	SpectrumColors []string    `json:"spectrumColors,omitempty" db:"-"` // For spectrum method
	ProcessingTime int64       `json:"processingTimeMs,omitempty" db:"processing_time_ms"`
	CreatedAt      time.Time   `json:"createdAt" db:"created_at"`
	CompletedAt    *time.Time  `json:"completedAt,omitempty" db:"completed_at"`
	ExpiresAt      time.Time   `json:"expiresAt" db:"expires_at"`
}

// GazePoint represents a point for gaze tracking
type GazePoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// LivenessStatus constants
const (
	LivenessStatusPending    = "Pending"
	LivenessStatusProcessing = "Processing"
	LivenessStatusPassed     = "Passed"
	LivenessStatusFailed     = "Failed"
	LivenessStatusExpired    = "Expired"
	// Aliases for backward compatibility
	LivenessStatusCreated   = "Pending"
	LivenessStatusCompleted = "Passed"
)

// LivenessAction represents an action for active liveness
type LivenessAction string

const (
	LivenessActionBlink     LivenessAction = "blink"
	LivenessActionSmile     LivenessAction = "smile"
	LivenessActionNod       LivenessAction = "nod"
	LivenessActionTurnRight LivenessAction = "turnRight"
	LivenessActionTurnLeft  LivenessAction = "turnLeft"
)

// CreateSessionRequest represents request to create liveness session
type CreateSessionRequest struct {
	NIK         string         `json:"nik" binding:"required,len=16"`
	Method      LivenessMethod `json:"method"`      // Default: passive
	RedirectURL string         `json:"redirectUrl"` // URL to redirect after Tencent flow
}

// CreateSessionResponse represents response after creating session
type CreateSessionResponse struct {
	SessionID string         `json:"sessionId"`
	NIK       string         `json:"nik"`
	Method    LivenessMethod `json:"method"`
	// BizToken       string         `json:"bizToken"` -- Removed for AWS
	// LivenessURL    string         `json:"livenessUrl"` -- Removed for AWS
	Challenges     []string    `json:"challenges,omitempty"`     // For active method
	GazePoints     []GazePoint `json:"gazePoints,omitempty"`     // For gaze method
	SpectrumColors []string    `json:"spectrumColors,omitempty"` // For spectrum method
	ExpiresAt      string      `json:"expiresAt"`
}

// VerifyLivenessRequest represents request to verify liveness with frames
// VerifyLivenessRequest represents request to verify liveness
type VerifyLivenessRequest struct {
	SessionID string          `json:"sessionId" binding:"required"`
	BizToken  string          `json:"bizToken"` // Optional if already in session, but good for validation
	Frames    []LivenessFrame `json:"frames"`   // Legacy/Custom frames (optional now)
}

// LivenessFrame represents a captured frame from frontend
type LivenessFrame struct {
	Timestamp int64    `json:"timestamp"`
	Action    string   `json:"action,omitempty"`  // For active: blink, smile, nod, turnRight, turnLeft
	Image     string   `json:"image"`             // Base64 encoded image (JPEG)
	FaceBox   *FaceBox `json:"faceBox,omitempty"` // Face bounding box from frontend detection
}

// FaceBox represents face bounding box coordinates
type FaceBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// LivenessResultResponse represents liveness result response
type LivenessResultResponse struct {
	SessionID  string          `json:"sessionId"`
	NIK        string          `json:"nik"`
	Method     LivenessMethod  `json:"method"`
	IsLive     bool            `json:"isLive"`
	Confidence float64         `json:"confidence"`
	File       LivenessFileURL `json:"file,omitempty"`
}

// LivenessFileURL represents file URLs for liveness
type LivenessFileURL struct {
	Face string `json:"face"`
}

// LivenessFailedResponse represents failed liveness response
type LivenessFailedResponse struct {
	SessionID     string         `json:"sessionId"`
	NIK           string         `json:"nik"`
	Method        LivenessMethod `json:"method"`
	IsLive        bool           `json:"isLive"`
	Confidence    float64        `json:"confidence"`
	FailureReason string         `json:"failureReason"`
	ErrorCode     string         `json:"errorCode"`
}

// LivenessErrorCode constants
const (
	LivenessErrNoFaceDetected     = "NO_FACE_DETECTED"
	LivenessErrMultipleFaces      = "MULTIPLE_FACES"
	LivenessErrLivenessFailed     = "LIVENESS_FAILED"
	LivenessErrActionNotCompleted = "ACTION_NOT_COMPLETED"
	LivenessErrGazeNotTracked     = "GAZE_NOT_TRACKED"
	LivenessErrSpoofDetected      = "SPOOF_DETECTED"
	LivenessErrSessionExpired     = "SESSION_EXPIRED"
	LivenessErrTimeout            = "TIMEOUT"
	LivenessErrInvalidSession     = "INVALID_SESSION"
)

// GetResultRequest represents request to get liveness result (backward compat)
type GetResultRequest struct {
	NIK       string `json:"nik" binding:"required,len=16"`
	SessionID string `json:"sessionId" binding:"required"`
}

// AWSLivenessResult represents AWS Rekognition liveness result (keep for reference)
type AWSLivenessResult struct {
	Status              string  `json:"status"`
	Confidence          float64 `json:"confidence"`
	ReferenceImageBytes string  `json:"referenceImageBytes,omitempty"`
	ReferenceImage      struct {
		S3Object struct {
			Bucket string `json:"bucket"`
			Key    string `json:"key"`
		} `json:"s3Object"`
		BoundingBox struct {
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
			Left   float64 `json:"left"`
			Top    float64 `json:"top"`
		} `json:"boundingBox"`
	} `json:"referenceImage"`
}
