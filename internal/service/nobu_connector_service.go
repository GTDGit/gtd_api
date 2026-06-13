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
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/models"
)

// Nobu QRIS connector: Nobu pulls a B2B access token from us (asymmetric
// SHA256withRSA over "clientKey|timestamp"), then calls our notify endpoint
// signed with HMAC-SHA512 (SNAP string-to-sign). This mirrors the BNC/BRI
// connector pattern; crypto is inline per the per-provider convention.

const (
	nobuConnectorTokenExpirySeconds = 900
	nobuNotifyScope                 = "nobu.qris.notify"
)

type NobuConnectorError struct {
	HTTPStatus      int
	ResponseCode    string
	ResponseMessage string
	Err             error
}

func (e *NobuConnectorError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.ResponseCode + ": " + e.ResponseMessage
	}
	return e.ResponseCode + ": " + e.ResponseMessage + ": " + e.Err.Error()
}

func (e *NobuConnectorError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type NobuConnectorTokenResponse struct {
	ResponseCode    string         `json:"responseCode"`
	ResponseMessage string         `json:"responseMessage"`
	AccessToken     string         `json:"accessToken"`
	TokenType       string         `json:"tokenType"`
	ExpiresIn       string         `json:"expiresIn"`
	AdditionalInfo  map[string]any `json:"additionalInfo"`
}

type NobuAckResponse struct {
	ResponseCode    string         `json:"responseCode"`
	ResponseMessage string         `json:"responseMessage"`
	AdditionalInfo  map[string]any `json:"additionalInfo"`
}

type nobuConnectorTokenRequest struct {
	GrantType string `json:"grantType"`
}

type nobuConnectorClaims struct {
	Connector string `json:"connector"`
	Scope     string `json:"scope"`
	jwt.RegisteredClaims
}

// nobuNotifyPayload is the Service 52 QRIS notify body. externalStoreId carries
// the NMID and is the merchant identification key for static QRIS.
type nobuNotifyPayload struct {
	OriginalReferenceNo        string         `json:"originalReferenceNo"`
	OriginalPartnerReferenceNo string         `json:"originalPartnerReferenceNo"`
	LatestTransactionStatus    string         `json:"latestTransactionStatus"`
	TransactionStatusDesc      string         `json:"transactionStatusDesc"`
	Amount                     nobuAmount     `json:"amount"`
	FeeAmount                  nobuAmount     `json:"feeAmount"`
	ExternalStoreID            string         `json:"externalStoreId"`
	AdditionalInfo             nobuNotifyInfo `json:"additionalInfo"`
}

type nobuAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type nobuNotifyInfo struct {
	CallbackURL          string `json:"callbackUrl"`
	IssuerID             string `json:"issuerId"`
	MerchantID           string `json:"merchantId"`
	SubMerchantID        string `json:"subMerchantId"`
	PayerName            string `json:"payerName"`
	PayerPhoneNumber     string `json:"payerPhoneNumber"`
	PaymentDate          string `json:"paymentDate"`
	RetrievalReferenceNo string `json:"retrievalReferenceNo"`
	PaymentReferenceNo   string `json:"paymentReferenceNo"`
	NettAmount           string `json:"nettAmount"`
	TerminalID           string `json:"terminalId"`
}

type NobuConnectorService struct {
	qrisPaymentService *QRISPaymentService
	jwtSecret          []byte
	clientSecret       string
	expectedClientKey  string
	inboundPublicKey   *rsa.PublicKey
	skipTokenSignature bool
}

func NewNobuConnectorService(
	qrisPaymentService *QRISPaymentService,
	jwtSecret string,
	clientSecret string,
	expectedClientKey string,
	connectorPublicKeyPEM string,
	connectorPublicKeyPath string,
	nobuEnv string,
) *NobuConnectorService {
	publicKey, err := loadConnectorPublicKey(connectorPublicKeyPEM, connectorPublicKeyPath)
	if err != nil {
		log.Warn().Err(err).Msg("failed to load Nobu connector public key; token signature verification disabled")
	}
	return &NobuConnectorService{
		qrisPaymentService: qrisPaymentService,
		jwtSecret:          []byte(strings.TrimSpace(jwtSecret)),
		clientSecret:       strings.TrimSpace(clientSecret),
		expectedClientKey:  strings.TrimSpace(expectedClientKey),
		inboundPublicKey:   publicKey,
		skipTokenSignature: strings.EqualFold(strings.TrimSpace(nobuEnv), "sandbox") && publicKey == nil,
	}
}

// IssueAccessToken validates Nobu's asymmetric signature and mints a short-lived
// HS256 JWT that Nobu must present on the subsequent notify call.
func (s *NobuConnectorService) IssueAccessToken(_ context.Context, headers http.Header, body []byte) (*NobuConnectorTokenResponse, error) {
	if len(s.jwtSecret) == 0 {
		return nil, newNobuConnectorError(http.StatusServiceUnavailable, "5007300", "Connector token service unavailable", nil)
	}

	var req nobuConnectorTokenRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, newNobuConnectorError(http.StatusBadRequest, "4007300", "Invalid request body", err)
	}
	if strings.TrimSpace(req.GrantType) != "client_credentials" {
		return nil, newNobuConnectorError(http.StatusBadRequest, "4007301", "Invalid grantType", nil)
	}

	timestamp := strings.TrimSpace(headers.Get("X-TIMESTAMP"))
	clientKey := strings.TrimSpace(headers.Get("X-CLIENT-KEY"))
	signature := strings.TrimSpace(headers.Get("X-SIGNATURE"))
	if timestamp == "" || clientKey == "" || signature == "" {
		return nil, newNobuConnectorError(http.StatusBadRequest, "4007302", "Missing required headers", nil)
	}
	if s.expectedClientKey != "" && clientKey != s.expectedClientKey {
		return nil, newNobuConnectorError(http.StatusUnauthorized, "4017300", "Unauthorized. Invalid client key", nil)
	}
	if err := validateSNAPTimestamp(timestamp, 5*time.Minute); err != nil {
		return nil, newNobuConnectorError(http.StatusUnauthorized, "4017301", "Unauthorized. Invalid X-TIMESTAMP", err)
	}
	if err := s.verifyTokenSignature(clientKey, timestamp, signature); err != nil {
		return nil, newNobuConnectorError(http.StatusUnauthorized, "4017302", "Unauthorized. Invalid X-SIGNATURE", err)
	}

	now := time.Now()
	claims := nobuConnectorClaims{
		Connector: "nobu",
		Scope:     nobuNotifyScope,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "gtd_api",
			Subject:   "nobu_connector",
			Audience:  jwt.ClaimStrings{nobuNotifyScope},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(nobuConnectorTokenExpirySeconds * time.Second)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, newNobuConnectorError(http.StatusInternalServerError, "5007301", "Failed to issue access token", err)
	}

	return &NobuConnectorTokenResponse{
		ResponseCode:    "2007300",
		ResponseMessage: "Request has been processed successfully",
		AccessToken:     accessToken,
		TokenType:       "Bearer",
		ExpiresIn:       fmt.Sprintf("%d", nobuConnectorTokenExpirySeconds),
		AdditionalInfo:  map[string]any{},
	}, nil
}

// HandleNotify validates the inbound QRIS notify (token + HMAC), then records the
// successful payment. It always returns a nil error to the caller on a duplicate
// or unknown merchant so the handler can ACK 200 and stop Nobu's retries.
func (s *NobuConnectorService) HandleNotify(ctx context.Context, headers http.Header, path string, body []byte) error {
	if s.qrisPaymentService == nil {
		return newNobuConnectorError(http.StatusServiceUnavailable, "5005200", "QRIS notify service unavailable", nil)
	}

	var payload nobuNotifyPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return newNobuConnectorError(http.StatusBadRequest, "4005200", "Invalid request body", err)
	}

	authHeader := strings.TrimSpace(headers.Get("Authorization"))
	accessToken := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	accessToken = strings.TrimSpace(strings.TrimPrefix(accessToken, "bearer"))
	timestamp := strings.TrimSpace(headers.Get("X-TIMESTAMP"))
	signature := strings.TrimSpace(headers.Get("X-SIGNATURE"))

	if timestamp == "" || signature == "" || accessToken == "" {
		return newNobuConnectorError(http.StatusUnauthorized, "4015200", "Unauthorized. Missing authentication headers", nil)
	}
	if err := validateSNAPTimestamp(timestamp, 5*time.Minute); err != nil {
		return newNobuConnectorError(http.StatusUnauthorized, "4015201", "Unauthorized. Invalid X-TIMESTAMP", err)
	}
	if err := s.validateAccessToken(accessToken); err != nil {
		return newNobuConnectorError(http.StatusUnauthorized, "4015202", "Unauthorized. Invalid token", err)
	}
	if err := s.verifyNotificationSignature(http.MethodPost, path, accessToken, timestamp, body, signature); err != nil {
		return newNobuConnectorError(http.StatusUnauthorized, "4015203", "Unauthorized. Invalid X-SIGNATURE", err)
	}

	// Only success notifications carry a payment to record. v1.4.5 removed the
	// "06 Failed" status from notify, so anything non-success is acknowledged
	// without persisting.
	if strings.TrimSpace(payload.LatestTransactionStatus) != "00" {
		log.Info().
			Str("status", payload.LatestTransactionStatus).
			Str("reference_no", payload.OriginalReferenceNo).
			Msg("nobu qris notify is non-success; acknowledging without recording")
		return nil
	}

	amount, _ := parseSNAPAmount(payload.Amount.Value)
	fee, _ := parseSNAPAmount(payload.FeeAmount.Value)
	nett, _ := parseSNAPAmount(payload.AdditionalInfo.NettAmount)

	event := QRISPaymentEvent{
		Provider:           models.QRISProviderNobu,
		StoreID:            strings.TrimSpace(payload.ExternalStoreID),
		ReferenceNo:        strings.TrimSpace(payload.OriginalReferenceNo),
		PartnerReferenceNo: payload.OriginalPartnerReferenceNo,
		RRN:                payload.AdditionalInfo.RetrievalReferenceNo,
		PaymentReferenceNo: payload.AdditionalInfo.PaymentReferenceNo,
		IssuerID:           payload.AdditionalInfo.IssuerID,
		TerminalID:         payload.AdditionalInfo.TerminalID,
		Amount:             amount,
		FeeAmount:          fee,
		NettAmount:         nett,
		PayerName:          payload.AdditionalInfo.PayerName,
		PayerPhone:         payload.AdditionalInfo.PayerPhoneNumber,
		PaidAt:             parseNobuPaymentDate(payload.AdditionalInfo.PaymentDate),
		RawPayload:         body,
	}

	_, _, err := s.qrisPaymentService.RecordQRISPayment(ctx, event)
	if err != nil {
		var svcErr *QRISPaymentServiceError
		if errors.As(err, &svcErr) && svcErr.HTTPStatus == http.StatusNotFound {
			// Unknown merchant: ACK 200 so Nobu stops retrying, but log it.
			log.Warn().
				Str("external_store_id", payload.ExternalStoreID).
				Str("reference_no", payload.OriginalReferenceNo).
				Msg("nobu qris notify for unknown merchant; acknowledged")
			return nil
		}
		return newNobuConnectorError(http.StatusInternalServerError, "5005201", "Failed to record QRIS payment", err)
	}
	return nil
}

func (s *NobuConnectorService) verifyTokenSignature(clientKey, timestamp, signature string) error {
	if s.inboundPublicKey == nil {
		if s.skipTokenSignature {
			log.Warn().Msg("Nobu connector token signature verification skipped in non-production")
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

func (s *NobuConnectorService) validateAccessToken(accessToken string) error {
	if len(s.jwtSecret) == 0 {
		return errors.New("connector token secret is not configured")
	}
	token, err := jwt.ParseWithClaims(accessToken, &nobuConnectorClaims{}, func(token *jwt.Token) (any, error) {
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
	claims, ok := token.Claims.(*nobuConnectorClaims)
	if !ok || !token.Valid {
		return errors.New("invalid token")
	}
	if claims.Connector != "nobu" || claims.Scope != nobuNotifyScope {
		return errors.New("invalid token scope")
	}
	return nil
}

func (s *NobuConnectorService) verifyNotificationSignature(method, path, accessToken, timestamp string, body []byte, signature string) error {
	if s.clientSecret == "" {
		return errors.New("nobu client secret is not configured")
	}
	bodyHash := sha256.Sum256(minifyJSONBody(body))
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

func newNobuConnectorError(httpStatus int, responseCode, responseMessage string, err error) *NobuConnectorError {
	return &NobuConnectorError{
		HTTPStatus:      httpStatus,
		ResponseCode:    responseCode,
		ResponseMessage: responseMessage,
		Err:             err,
	}
}

// parseSNAPAmount turns a SNAP money string ("10000.00") into an int64 of whole
// currency units, dropping the fractional part.
func parseSNAPAmount(v string) (int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	if dot := strings.Index(v, "."); dot >= 0 {
		v = v[:dot]
	}
	var n int64
	_, err := fmt.Sscan(v, &n)
	return n, err
}

// parseNobuPaymentDate parses Nobu's "2006-01-02 15:04:05" payment date (WIB).
func parseNobuPaymentDate(v string) *time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	loc := time.FixedZone("WIB", 7*3600)
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", v, loc); err == nil {
		return &t
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t
	}
	return nil
}

// minifyJSONBody strips insignificant whitespace outside of JSON string literals,
// matching the SNAP "minify(RequestBody)" expectation. Falls back to the raw
// bytes if the body is not valid JSON.
func minifyJSONBody(body []byte) []byte {
	var compact []byte
	out := make([]byte, 0, len(body))
	inString := false
	escaped := false
	for _, b := range body {
		if inString {
			out = append(out, b)
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				inString = false
			}
			continue
		}
		switch b {
		case ' ', '\n', '\t', '\r':
			continue
		case '"':
			inString = true
		}
		out = append(out, b)
	}
	compact = out
	if !json.Valid(body) {
		return body
	}
	return compact
}
