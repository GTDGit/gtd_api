package service

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

const (
	bncConnectorTokenExpirySeconds = 900
	bncTransferNotifyScope         = "bnc.transfer.notify"
)

type BNCConnectorError struct {
	HTTPStatus      int
	ResponseCode    string
	ResponseMessage string
	Err             error
}

func (e *BNCConnectorError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.ResponseCode + ": " + e.ResponseMessage
	}
	return e.ResponseCode + ": " + e.ResponseMessage + ": " + e.Err.Error()
}

func (e *BNCConnectorError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type BNCConnectorTokenResponse struct {
	ResponseCode    string `json:"responseCode"`
	ResponseMessage string `json:"responseMessage"`
	AccessToken     string `json:"accessToken"`
	TokenType       string `json:"tokenType"`
	ExpiresIn       string `json:"expiresIn"`
}

type BNCAckResponse struct {
	ResponseCode    string `json:"responseCode"`
	ResponseMessage string `json:"responseMessage"`
}

type bncConnectorTokenRequest struct {
	GrantType string `json:"grantType"`
}

type bncConnectorClaims struct {
	Connector string `json:"connector"`
	Scope     string `json:"scope"`
	jwt.RegisteredClaims
}

type BNCConnectorService struct {
	transferRepo       *repository.TransferRepository
	transferService    *TransferService
	jwtSecret          []byte
	clientSecret       string
	expectedClientKey  string
	inboundPublicKey   *rsa.PublicKey
	skipTokenSignature bool
}

func NewBNCConnectorService(
	transferRepo *repository.TransferRepository,
	transferService *TransferService,
	jwtSecret string,
	clientSecret string,
	expectedClientKey string,
	connectorPublicKeyPEM string,
	connectorPublicKeyPath string,
	bncEnv string,
) *BNCConnectorService {
	publicKey, err := loadConnectorPublicKey(connectorPublicKeyPEM, connectorPublicKeyPath)
	if err != nil {
		log.Warn().Err(err).Msg("failed to load BNC connector public key; token signature verification disabled")
	}

	return &BNCConnectorService{
		transferRepo:       transferRepo,
		transferService:    transferService,
		jwtSecret:          []byte(strings.TrimSpace(jwtSecret)),
		clientSecret:       strings.TrimSpace(clientSecret),
		expectedClientKey:  strings.TrimSpace(expectedClientKey),
		inboundPublicKey:   publicKey,
		skipTokenSignature: strings.EqualFold(strings.TrimSpace(bncEnv), "sandbox") && publicKey == nil,
	}
}

func (s *BNCConnectorService) IssueAccessToken(
	_ context.Context,
	headers http.Header,
	body []byte,
) (*BNCConnectorTokenResponse, error) {
	if len(s.jwtSecret) == 0 {
		return nil, newBNCConnectorError(http.StatusServiceUnavailable, "5037300", "Connector token service unavailable", nil)
	}

	var req bncConnectorTokenRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, newBNCConnectorError(http.StatusBadRequest, "4007300", "Invalid request body", err)
	}
	if strings.TrimSpace(req.GrantType) != "client_credentials" {
		return nil, newBNCConnectorError(http.StatusBadRequest, "4007301", "Invalid grantType", nil)
	}

	timestamp := strings.TrimSpace(headers.Get("X-TIMESTAMP"))
	clientKey := strings.TrimSpace(headers.Get("X-CLIENT-KEY"))
	signature := strings.TrimSpace(headers.Get("X-SIGNATURE"))

	if timestamp == "" || clientKey == "" || signature == "" {
		return nil, newBNCConnectorError(http.StatusBadRequest, "4007302", "Missing required headers", nil)
	}
	if s.expectedClientKey != "" && clientKey != s.expectedClientKey {
		return nil, newBNCConnectorError(http.StatusUnauthorized, "4017300", "Unauthorized. Invalid client key", nil)
	}
	if err := validateSNAPTimestamp(timestamp, 5*time.Minute); err != nil {
		return nil, newBNCConnectorError(http.StatusUnauthorized, "4017301", "Unauthorized. Invalid X-TIMESTAMP", err)
	}
	if err := s.verifyTokenSignature(clientKey, timestamp, signature); err != nil {
		return nil, newBNCConnectorError(http.StatusUnauthorized, "4017302", "Unauthorized. Invalid X-SIGNATURE", err)
	}

	now := time.Now()
	claims := bncConnectorClaims{
		Connector: "bnc",
		Scope:     bncTransferNotifyScope,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "gtd_api",
			Subject:   "bnc_connector",
			Audience:  jwt.ClaimStrings{bncTransferNotifyScope},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(bncConnectorTokenExpirySeconds * time.Second)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, newBNCConnectorError(http.StatusInternalServerError, "5007300", "Failed to issue access token", err)
	}

	return &BNCConnectorTokenResponse{
		ResponseCode:    "2007300",
		ResponseMessage: "Successful",
		AccessToken:     accessToken,
		TokenType:       "Bearer",
		ExpiresIn:       fmt.Sprintf("%d", bncConnectorTokenExpirySeconds),
	}, nil
}

func (s *BNCConnectorService) HandleTransferNotify(
	ctx context.Context,
	headers http.Header,
	path string,
	body []byte,
) error {
	if s.transferRepo == nil || s.transferService == nil {
		return newBNCConnectorError(http.StatusServiceUnavailable, "5032500", "Transfer notification service unavailable", nil)
	}

	var payload BNCTransferNotification
	if err := json.Unmarshal(body, &payload); err != nil {
		return newBNCConnectorError(http.StatusBadRequest, "4002500", "Invalid request body", err)
	}

	authHeader := strings.TrimSpace(headers.Get("Authorization"))
	accessToken := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	accessToken = strings.TrimSpace(strings.TrimPrefix(accessToken, "bearer"))
	timestamp := strings.TrimSpace(headers.Get("X-TIMESTAMP"))
	signature := strings.TrimSpace(headers.Get("X-SIGNATURE"))

	callback := &models.TransferCallback{
		Provider:         models.DisbursementProviderBNC,
		ProviderRef:      stringPtr(payload.ProviderRef()),
		Headers:          marshalHeaders(headers),
		Payload:          models.NullableRawMessage(body),
		Signature:        stringPtr(signature),
		IsValidSignature: false,
		TransferID:       stringPtr(payload.TransferID()),
		Status:           stringPtr(strings.TrimSpace(payload.LatestTransactionStatus)),
		IsProcessed:      false,
	}
	if err := s.transferRepo.CreateTransferCallback(ctx, callback); err != nil {
		return newBNCConnectorError(http.StatusInternalServerError, "5002500", "Failed to persist callback", err)
	}

	processErr := func(err error, processed bool) error {
		msg := stringPtr(err.Error())
		if updateErr := s.transferRepo.UpdateTransferCallbackProcessed(ctx, callback.ID, processed, msg); updateErr != nil {
			log.Warn().Err(updateErr).Int("callback_id", callback.ID).Msg("failed to update transfer callback status")
		}
		return err
	}

	if timestamp == "" || signature == "" || accessToken == "" {
		return processErr(newBNCConnectorError(http.StatusUnauthorized, "4012500", "Unauthorized. Missing authentication headers", nil), true)
	}
	if err := validateSNAPTimestamp(timestamp, 5*time.Minute); err != nil {
		return processErr(newBNCConnectorError(http.StatusUnauthorized, "4012501", "Unauthorized. Invalid X-TIMESTAMP", err), true)
	}
	if err := s.validateAccessToken(accessToken); err != nil {
		return processErr(newBNCConnectorError(http.StatusUnauthorized, "4012502", "Unauthorized. Invalid token", err), true)
	}
	if err := s.verifyNotificationSignature(http.MethodPost, path, accessToken, timestamp, body, signature); err != nil {
		return processErr(newBNCConnectorError(http.StatusUnauthorized, "4012503", "Unauthorized. Invalid X-SIGNATURE", err), true)
	}

	callback.IsValidSignature = true
	if err := s.transferRepo.UpdateTransferCallbackSignature(ctx, callback.ID, true); err != nil {
		log.Warn().Err(err).Int("callback_id", callback.ID).Msg("failed to update transfer callback signature status")
	}
	if err := s.transferRepo.UpdateTransferCallbackProcessed(ctx, callback.ID, false, nil); err != nil {
		log.Warn().Err(err).Int("callback_id", callback.ID).Msg("failed to clear transfer callback processing error")
	}

	if err := s.transferService.ApplyBNCNotification(ctx, &payload, body); err != nil {
		var transferErr *TransferServiceError
		if errors.As(err, &transferErr) && transferErr.HTTPStatus == http.StatusNotFound {
			return processErr(newBNCConnectorError(http.StatusNotFound, "4042501", "Transfer not found", err), true)
		}
		return processErr(newBNCConnectorError(http.StatusInternalServerError, "5002501", "Failed to process transfer notification", err), true)
	}

	if err := s.transferRepo.UpdateTransferCallbackProcessed(ctx, callback.ID, true, nil); err != nil {
		log.Warn().Err(err).Int("callback_id", callback.ID).Msg("failed to mark transfer callback as processed")
	}

	return nil
}

func (s *BNCConnectorService) verifyTokenSignature(clientKey, timestamp, signature string) error {
	if s.inboundPublicKey == nil {
		if s.skipTokenSignature {
			log.Warn().Msg("BNC connector token signature verification skipped in non-production")
			return nil
		}
		return errors.New("connector public key is not configured")
	}

	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return err
	}

	payload := clientKey + "|" + timestamp
	hash := sha256.Sum256([]byte(payload))
	return rsa.VerifyPKCS1v15(s.inboundPublicKey, crypto.SHA256, hash[:], sig)
}

func (s *BNCConnectorService) validateAccessToken(accessToken string) error {
	if len(s.jwtSecret) == 0 {
		return errors.New("connector token secret is not configured")
	}

	token, err := jwt.ParseWithClaims(accessToken, &bncConnectorClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method == nil {
			return nil, errors.New("missing signing method")
		}
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method %s", token.Method.Alg())
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return err
	}

	claims, ok := token.Claims.(*bncConnectorClaims)
	if !ok || !token.Valid {
		return errors.New("invalid token")
	}
	if claims.Connector != "bnc" || claims.Scope != bncTransferNotifyScope {
		return errors.New("invalid token scope")
	}
	return nil
}

func (s *BNCConnectorService) verifyNotificationSignature(
	method string,
	path string,
	accessToken string,
	timestamp string,
	body []byte,
	signature string,
) error {
	if s.clientSecret == "" {
		return errors.New("bnc client secret is not configured")
	}

	bodyHash := sha256.Sum256(body)
	stringToSign := strings.Join([]string{
		method,
		path,
		accessToken,
		strings.ToLower(hex.EncodeToString(bodyHash[:])),
		timestamp,
	}, ":")

	mac := hmac.New(sha512.New, []byte(s.clientSecret))
	mac.Write([]byte(stringToSign))
	expectedSignature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expectedSignature), []byte(signature)) {
		return errors.New("signature mismatch")
	}
	return nil
}

func validateSNAPTimestamp(raw string, maxSkew time.Duration) error {
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return err
	}
	diff := time.Since(ts)
	if diff < 0 {
		diff = -diff
	}
	if maxSkew > 0 && diff > maxSkew {
		return fmt.Errorf("timestamp outside allowed skew: %s", diff)
	}
	return nil
}

func loadConnectorPublicKey(pemValue, path string) (*rsa.PublicKey, error) {
	trimmedPEM := strings.TrimSpace(pemValue)
	if trimmedPEM == "" && strings.TrimSpace(path) != "" {
		raw, err := os.ReadFile(strings.TrimSpace(path))
		if err != nil {
			return nil, err
		}
		trimmedPEM = string(raw)
	}
	if trimmedPEM == "" {
		return nil, nil
	}

	block, _ := pem.Decode([]byte(trimmedPEM))
	if block == nil {
		return nil, errors.New("failed to decode public key PEM")
	}

	if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rsaPub, ok := pub.(*rsa.PublicKey); ok {
			return rsaPub, nil
		}
	}

	if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
		if rsaPub, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			return rsaPub, nil
		}
	}

	return nil, errors.New("public key is not valid RSA key material")
}

func marshalHeaders(headers http.Header) models.NullableRawMessage {
	if len(headers) == 0 {
		return nil
	}
	raw, err := json.Marshal(headers)
	if err != nil {
		return nil
	}
	return models.NullableRawMessage(raw)
}

func newBNCConnectorError(httpStatus int, responseCode, responseMessage string, err error) *BNCConnectorError {
	return &BNCConnectorError{
		HTTPStatus:      httpStatus,
		ResponseCode:    responseCode,
		ResponseMessage: responseMessage,
		Err:             err,
	}
}
