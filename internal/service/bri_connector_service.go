package service

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

const (
	briConnectorTokenExpirySeconds = 900
	briVANotifyScope               = "bri.va.notify"
)

type BRIConnectorError struct {
	HTTPStatus      int
	ResponseCode    string
	ResponseMessage string
	Err             error
}

func (e *BRIConnectorError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.ResponseCode + ": " + e.ResponseMessage
	}
	return e.ResponseCode + ": " + e.ResponseMessage + ": " + e.Err.Error()
}

func (e *BRIConnectorError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type BRIConnectorTokenResponse struct {
	ResponseCode    string `json:"responseCode"`
	ResponseMessage string `json:"responseMessage"`
	AccessToken     string `json:"accessToken"`
	TokenType       string `json:"tokenType"`
	ExpiresIn       string `json:"expiresIn"`
}

type BRIAckResponse struct {
	ResponseCode    string `json:"responseCode"`
	ResponseMessage string `json:"responseMessage"`
}

type briConnectorTokenRequest struct {
	GrantType string `json:"grantType"`
}

type briConnectorClaims struct {
	Connector string `json:"connector"`
	Scope     string `json:"scope"`
	jwt.RegisteredClaims
}

type BRIVAPaymentNotification struct {
	PartnerServiceID string                 `json:"partnerServiceId"`
	CustomerNo       string                 `json:"customerNo"`
	VirtualAccountNo string                 `json:"virtualAccountNo"`
	PaymentRequestID string                 `json:"paymentRequestId"`
	TrxDateTime      string                 `json:"trxDateTime"`
	AdditionalInfo   map[string]any         `json:"additionalInfo"`
}

func (n *BRIVAPaymentNotification) ProviderRef() string {
	return strings.TrimSpace(n.PaymentRequestID)
}

func (n *BRIVAPaymentNotification) PaymentIdentifier() string {
	return strings.TrimSpace(n.PaymentRequestID)
}

func (n *BRIVAPaymentNotification) PaymentAmount() *int64 {
	if n == nil || n.AdditionalInfo == nil {
		return nil
	}

	switch v := n.AdditionalInfo["paymentAmount"].(type) {
	case float64:
		value := int64(v)
		return &value
	case string:
		normalized := strings.TrimSpace(v)
		normalized = strings.ReplaceAll(normalized, ".00", "")
		normalized = strings.ReplaceAll(normalized, ".", "")
		normalized = strings.ReplaceAll(normalized, ",", "")
		if normalized == "" {
			return nil
		}
		value, err := strconv.ParseInt(normalized, 10, 64)
		if err == nil {
			return &value
		}
	}
	return nil
}

type BRIConnectorService struct {
	paymentRepo         *repository.PaymentRepository
	jwtSecret           []byte
	clientSecret        string
	expectedClientKey   string
	inboundPublicKey    *rsa.PublicKey
	skipTokenSignature  bool
}

func NewBRIConnectorService(
	paymentRepo *repository.PaymentRepository,
	jwtSecret string,
	clientSecret string,
	expectedClientKey string,
	connectorPublicKeyPEM string,
	connectorPublicKeyPath string,
	briEnv string,
) *BRIConnectorService {
	publicKey, err := loadConnectorPublicKey(connectorPublicKeyPEM, connectorPublicKeyPath)
	if err != nil {
		log.Warn().Err(err).Msg("failed to load BRI connector public key; token signature verification disabled")
	}

	return &BRIConnectorService{
		paymentRepo:        paymentRepo,
		jwtSecret:          []byte(strings.TrimSpace(jwtSecret)),
		clientSecret:       strings.TrimSpace(clientSecret),
		expectedClientKey:  strings.TrimSpace(expectedClientKey),
		inboundPublicKey:   publicKey,
		skipTokenSignature: strings.EqualFold(strings.TrimSpace(briEnv), "sandbox") && publicKey == nil,
	}
}

func (s *BRIConnectorService) IssueAccessToken(_ context.Context, headers http.Header, body []byte) (*BRIConnectorTokenResponse, error) {
	if len(s.jwtSecret) == 0 {
		return nil, newBRIConnectorError(http.StatusServiceUnavailable, "5037300", "Connector token service unavailable", nil)
	}

	var req briConnectorTokenRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, newBRIConnectorError(http.StatusBadRequest, "4007300", "Invalid request body", err)
	}
	if strings.TrimSpace(req.GrantType) != "client_credentials" {
		return nil, newBRIConnectorError(http.StatusBadRequest, "4007301", "Invalid grantType", nil)
	}

	timestamp := strings.TrimSpace(headers.Get("X-TIMESTAMP"))
	clientKey := strings.TrimSpace(headers.Get("X-CLIENT-KEY"))
	signature := strings.TrimSpace(headers.Get("X-SIGNATURE"))

	if timestamp == "" || clientKey == "" || signature == "" {
		return nil, newBRIConnectorError(http.StatusBadRequest, "4007302", "Missing required headers", nil)
	}
	if s.expectedClientKey != "" && clientKey != s.expectedClientKey {
		return nil, newBRIConnectorError(http.StatusUnauthorized, "4017300", "Unauthorized. Invalid client key", nil)
	}
	if err := validateSNAPTimestamp(timestamp, 5*time.Minute); err != nil {
		return nil, newBRIConnectorError(http.StatusUnauthorized, "4017301", "Unauthorized. Invalid X-TIMESTAMP", err)
	}
	if err := s.verifyTokenSignature(clientKey, timestamp, signature); err != nil {
		return nil, newBRIConnectorError(http.StatusUnauthorized, "4017302", "Unauthorized. Invalid X-SIGNATURE", err)
	}

	now := time.Now()
	claims := briConnectorClaims{
		Connector: "bri",
		Scope:     briVANotifyScope,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "gtd_api",
			Subject:   "bri_connector",
			Audience:  jwt.ClaimStrings{briVANotifyScope},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(briConnectorTokenExpirySeconds * time.Second)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, newBRIConnectorError(http.StatusInternalServerError, "5007300", "Failed to issue access token", err)
	}

	return &BRIConnectorTokenResponse{
		ResponseCode:    "2007300",
		ResponseMessage: "Successful",
		AccessToken:     accessToken,
		TokenType:       "Bearer",
		ExpiresIn:       fmt.Sprintf("%d", briConnectorTokenExpirySeconds),
	}, nil
}

func (s *BRIConnectorService) HandleVAPaymentNotify(ctx context.Context, headers http.Header, path string, body []byte) error {
	if s.paymentRepo == nil {
		return newBRIConnectorError(http.StatusServiceUnavailable, "5033400", "Payment notification service unavailable", nil)
	}

	var payload BRIVAPaymentNotification
	if err := json.Unmarshal(body, &payload); err != nil {
		return newBRIConnectorError(http.StatusBadRequest, "4003400", "Invalid request body", err)
	}

	authHeader := strings.TrimSpace(headers.Get("Authorization"))
	accessToken := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	accessToken = strings.TrimSpace(strings.TrimPrefix(accessToken, "bearer"))
	timestamp := strings.TrimSpace(headers.Get("X-TIMESTAMP"))
	signature := strings.TrimSpace(headers.Get("X-SIGNATURE"))

	callback := &models.PaymentCallback{
		Provider:         string(models.DisbursementProviderBRI),
		ProviderRef:      stringPtr(payload.ProviderRef()),
		Headers:          marshalHeaders(headers),
		Payload:          models.NullableRawMessage(body),
		Signature:        stringPtr(signature),
		IsValidSignature: false,
		PaymentID:        stringPtr(payload.PaymentIdentifier()),
		Status:           stringPtr("NOTIFIED"),
		PaidAmount:       payload.PaymentAmount(),
		IsProcessed:      false,
	}
	if err := s.paymentRepo.CreatePaymentCallback(ctx, callback); err != nil {
		return newBRIConnectorError(http.StatusInternalServerError, "5003400", "Failed to persist payment callback", err)
	}

	processErr := func(err error, processed bool) error {
		msg := stringPtr(err.Error())
		if updateErr := s.paymentRepo.UpdatePaymentCallbackProcessed(ctx, callback.ID, processed, msg); updateErr != nil {
			log.Warn().Err(updateErr).Int("callback_id", callback.ID).Msg("failed to update payment callback status")
		}
		return err
	}

	if timestamp == "" || signature == "" || accessToken == "" {
		return processErr(newBRIConnectorError(http.StatusUnauthorized, "4013400", "Unauthorized. Missing authentication headers", nil), true)
	}
	if err := validateSNAPTimestamp(timestamp, 5*time.Minute); err != nil {
		return processErr(newBRIConnectorError(http.StatusUnauthorized, "4013401", "Unauthorized. Invalid X-TIMESTAMP", err), true)
	}
	if err := s.validateAccessToken(accessToken); err != nil {
		return processErr(newBRIConnectorError(http.StatusUnauthorized, "4013402", "Unauthorized. Invalid token", err), true)
	}
	if err := s.verifyNotificationSignature(http.MethodPost, path, accessToken, timestamp, body, signature); err != nil {
		return processErr(newBRIConnectorError(http.StatusUnauthorized, "4013403", "Unauthorized. Invalid X-SIGNATURE", err), true)
	}

	callback.IsValidSignature = true
	if err := s.paymentRepo.UpdatePaymentCallbackSignature(ctx, callback.ID, true); err != nil {
		log.Warn().Err(err).Int("callback_id", callback.ID).Msg("failed to update payment callback signature status")
	}

	if err := s.paymentRepo.UpdatePaymentCallbackProcessed(ctx, callback.ID, true, nil); err != nil {
		log.Warn().Err(err).Int("callback_id", callback.ID).Msg("failed to mark payment callback as processed")
	}

	return nil
}

func (s *BRIConnectorService) verifyTokenSignature(clientKey, timestamp, signature string) error {
	if s.inboundPublicKey == nil {
		if s.skipTokenSignature {
			log.Warn().Msg("BRI connector token signature verification skipped in non-production")
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

func (s *BRIConnectorService) validateAccessToken(accessToken string) error {
	if len(s.jwtSecret) == 0 {
		return errors.New("connector token secret is not configured")
	}

	token, err := jwt.ParseWithClaims(accessToken, &briConnectorClaims{}, func(token *jwt.Token) (interface{}, error) {
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

	claims, ok := token.Claims.(*briConnectorClaims)
	if !ok || !token.Valid {
		return errors.New("invalid token")
	}
	if claims.Connector != "bri" || claims.Scope != briVANotifyScope {
		return errors.New("invalid token scope")
	}
	return nil
}

func (s *BRIConnectorService) verifyNotificationSignature(method, path, accessToken, timestamp string, body []byte, signature string) error {
	if s.clientSecret == "" {
		return errors.New("bri client secret is not configured")
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

func newBRIConnectorError(httpStatus int, responseCode, responseMessage string, err error) *BRIConnectorError {
	return &BRIConnectorError{
		HTTPStatus:      httpStatus,
		ResponseCode:    responseCode,
		ResponseMessage: responseMessage,
		Err:             err,
	}
}
