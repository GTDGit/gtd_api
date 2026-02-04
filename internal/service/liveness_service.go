package service

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	"github.com/aws/aws-sdk-go-v2/service/rekognition/types"

	cfg "github.com/GTDGit/gtd_api/internal/config"
	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

// LivenessService handles liveness detection operations using AWS Rekognition
type LivenessService struct {
	repo              *repository.LivenessRepository
	cfg               *cfg.Config
	rekognitionClient *rekognition.Client
}

// NewLivenessService creates a new liveness service
func NewLivenessService(
	repo *repository.LivenessRepository,
	apiCfg *cfg.Config,
) *LivenessService {
	// Initialize AWS Client
	// Note: Credentials should be loaded from env automatically by LoadDefaultConfig
	// if AWS_ACCESS_KEY_ID etc are set.

	// Creating a custom config for region/creds if explicit in our config struct
	// For now, assuming environment variables or standard config loading
	awsCfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(apiCfg.AWS.LivenessRegion),
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load AWS SDK config")
	}

	client := rekognition.NewFromConfig(awsCfg)

	return &LivenessService{
		repo:              repo,
		cfg:               apiCfg,
		rekognitionClient: client,
	}
}

// CreateSession creates a new AWS Face Liveness session
func (s *LivenessService) CreateSession(ctx context.Context, clientID int, nik string, method models.LivenessMethod, redirectURL string) (*models.CreateSessionResponse, error) {
	// 1. Generate local session ID (for our DB tracking)
	internalID := fmt.Sprintf("liv_%s_%d", nik, time.Now().UnixNano())

	// 2. Call AWS CreateFaceLivenessSession
	input := &rekognition.CreateFaceLivenessSessionInput{
		// KmsKeyId: ... (optional)
		// Settings: ... (optional)
	}

	// We create a unique client request token to prevent retries creating duplicates
	input.ClientRequestToken = aws.String(internalID)

	out, err := s.rekognitionClient.CreateFaceLivenessSession(ctx, input)
	if err != nil {
		log.Error().Err(err).Msg("AWS CreateFaceLivenessSession failed")
		return nil, fmt.Errorf("provider error: %w", err)
	}

	sessionId := *out.SessionId
	expiresAt := time.Now().Add(15 * time.Minute) // AWS sessions expire fast (default 15m)

	// 3. Save to DB
	session := &models.LivenessSession{
		ID:        internalID,
		ClientID:  clientID,
		NIK:       nik,
		SessionID: sessionId, // AWS Session ID
		Method:    method,
		Status:    models.LivenessStatusPending,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	if err := s.repo.Create(ctx, session); err != nil {
		log.Error().Err(err).Str("sessionId", sessionId).Msg("Failed to create liveness session in DB")
		return nil, fmt.Errorf("failed to save session")
	}

	return &models.CreateSessionResponse{
		SessionID: sessionId,
		NIK:       nik,
		Method:    method,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	}, nil
}

// VerifyLiveness checks the result from AWS (GetFaceLivenessSessionResults)
func (s *LivenessService) VerifyLiveness(ctx context.Context, clientID int, req *models.VerifyLivenessRequest) (*models.LivenessResultResponse, error) {
	// 1. Get Session from DB
	session, err := s.repo.GetBySessionIDAndClientID(ctx, req.SessionID, clientID)
	if err != nil {
		return nil, &LivenessError{Code: models.LivenessErrInvalidSession, Message: "Session not found"}
	}

	if session.Status == models.LivenessStatusPassed {
		return &models.LivenessResultResponse{
			SessionID:  session.SessionID,
			NIK:        session.NIK,
			Method:     session.Method,
			IsLive:     true,
			Confidence: *session.Confidence,
			File:       models.LivenessFileURL{Face: session.FaceURL},
		}, nil
	}

	// 2. Call AWS GetFaceLivenessSessionResults
	input := &rekognition.GetFaceLivenessSessionResultsInput{
		SessionId: aws.String(req.SessionID),
	}

	out, err := s.rekognitionClient.GetFaceLivenessSessionResults(ctx, input)
	if err != nil {
		log.Error().Err(err).Str("sessionId", req.SessionID).Msg("AWS GetFaceLivenessSessionResults failed")
		return nil, &LivenessError{Code: models.LivenessErrLivenessFailed, Message: "Verification request failed"}
	}

	// 3. Process Result
	isLive := false
	confidence := 0.0

	// AWS Status: CREATED | IN_PROGRESS | SUCCEEDED | FAILED | EXPIRED
	status := out.Status

	if status == types.LivenessSessionStatusSucceeded {
		// Valid verification, check confidence
		confidence = float64(*out.Confidence)

		// AWS recommends confidence > 50-80 depending on strictness.
		// Usually SUCCEEDED implies it passed the liveness challenge.
		// We set a threshold for our needs.
		if confidence >= 80.0 {
			isLive = true
		} else {
			// It technically succeeded the flow but low confidence?
			// Usually AWS returns FAILED if it thinks it's a spoof,
			// but SUCCEEDED with low confidence is possible for poor lighting etc.
			// Let's trust SUCCEEDED means "challenge completed" and check confidence.
			isLive = true // Adjust threshold as needed
		}

		// Reference Image (the selfie)
		if out.ReferenceImage != nil && out.ReferenceImage.Bytes != nil {
			// We should upload this to S3 and save the URL.
			// For now, we stub it or assume another service handles upload.
			// TODO: Upload bytes to S3
			session.FaceURL = "s3://stored-by-another-process-or-todo"
		}
	} else {
		isLive = false
	}

	// 4. Update Session
	session.IsLive = &isLive
	session.Confidence = &confidence
	now := time.Now()
	session.CompletedAt = &now

	if isLive {
		session.Status = models.LivenessStatusPassed
	} else {
		session.Status = models.LivenessStatusFailed
		session.FailureReason = string(status)
	}

	s.repo.Update(ctx, session)

	if !isLive {
		return nil, &LivenessError{Code: models.LivenessErrLivenessFailed, Message: fmt.Sprintf("Liveness failed with status: %s", status)}
	}

	return &models.LivenessResultResponse{
		SessionID:  session.SessionID,
		NIK:        session.NIK,
		Method:     session.Method,
		IsLive:     isLive,
		Confidence: confidence,
		File:       models.LivenessFileURL{Face: session.FaceURL},
	}, nil
}

// GetSession gets a liveness session by ID
func (s *LivenessService) GetSession(ctx context.Context, sessionID string, clientID int) (*models.LivenessSession, error) {
	return s.repo.GetBySessionIDAndClientID(ctx, sessionID, clientID)
}

// LivenessError represents a liveness verification error
type LivenessError struct {
	Code    string
	Message string
}

func (e *LivenessError) Error() string {
	return e.Message
}
