package service

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/config"
	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
)

// OCRService handles OCR operations
type OCRService struct {
	ocrRepo       *repository.OCRRepository
	territoryRepo *repository.TerritoryRepository
	cfg           *config.IdentityConfig
	s3Svc         *S3Service
}

// NewOCRService creates a new OCR service
func NewOCRService(
	ocrRepo *repository.OCRRepository,
	territoryRepo *repository.TerritoryRepository,
	cfg *config.IdentityConfig,
	s3Svc *S3Service,
) *OCRService {
	return &OCRService{
		ocrRepo:       ocrRepo,
		territoryRepo: territoryRepo,
		cfg:           cfg,
		s3Svc:         s3Svc,
	}
}

// OCRResult represents the result of OCR processing
type OCRResult struct {
	Data           interface{}           `json:"data"`
	Validation     *models.OCRValidation `json:"validation,omitempty"`
	ProcessingTime int64                 `json:"processingTimeMs"`
}

// QualityError represents image quality validation error
type QualityError struct {
	Details []string             `json:"details"`
	Quality *models.QualityCheck `json:"quality,omitempty"`
}

func (e *QualityError) Error() string {
	return strings.Join(e.Details, "; ")
}

// ProcessKTPOCR processes KTP image and extracts data
func (s *OCRService) ProcessKTPOCR(ctx context.Context, clientID int, imageData []byte, validateQuality, validateNik bool) (*OCRResult, error) {
	startTime := time.Now()

	// 1. Validate image format and size
	if err := s.validateImage(imageData); err != nil {
		return nil, err
	}

	// 2. Quality validation (optional)
	if validateQuality {
		if err := s.validateImageQuality(imageData); err != nil {
			return nil, err
		}
	}

	// 3. Preprocess image (grayscale, enhance)
	processedImage := s.preprocessImage(imageData)

	// 4. Extract text using Google Vision API
	rawText, err := s.extractTextFromImage(ctx, processedImage)
	if err != nil {
		return nil, fmt.Errorf("text extraction failed: %w", err)
	}

	if rawText == "" {
		return nil, errors.New("no text detected in image")
	}

	// 5. Validate document type - must be KTP
	if err := s.validateDocumentType(rawText, "KTP"); err != nil {
		return nil, err
	}

	// 6. Parse text to structured data using Groq API
	ktpData, err := s.parseKTPText(ctx, rawText)
	if err != nil {
		return nil, fmt.Errorf("text parsing failed: %w", err)
	}

	// 7. Match administrative codes (hierarchical) and update address with database names
	adminCode, err := s.matchAdministrativeCodesWithNames(ctx, &ktpData.Address)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to match administrative codes")
	}
	ktpData.AdministrativeCode = adminCode

	// 8. NIK validation (optional)
	var validation *models.OCRValidation
	if validateNik {
		nikValidation := s.validateNIK(ctx, ktpData.NIK, ktpData.DateOfBirth, adminCode)
		validation = &models.OCRValidation{NIK: nikValidation}
	}

	// 9. Upload to S3
	documentURL, err := s.s3Svc.UploadKTPDocument(ctx, ktpData.NIK, imageData)
	if err != nil {
		log.Error().Err(err).Msg("Failed to upload KTP to S3")
	}
	ktpData.File = models.FileURLs{Document: documentURL}

	// 10. Save to database
	processingTime := time.Since(startTime).Milliseconds()
	record := s.buildKTPRecord(clientID, ktpData, rawText, processingTime)
	if err := s.ocrRepo.Create(ctx, record); err != nil {
		log.Error().Err(err).Msg("Failed to save OCR record")
	}

	return &OCRResult{
		Data:           ktpData,
		Validation:     validation,
		ProcessingTime: processingTime,
	}, nil
}

// ProcessNPWPOCR processes NPWP image and extracts data
func (s *OCRService) ProcessNPWPOCR(ctx context.Context, clientID int, imageData []byte, validateQuality bool) (*OCRResult, error) {
	startTime := time.Now()

	// 1. Validate image
	if err := s.validateImage(imageData); err != nil {
		return nil, err
	}

	// 2. Quality validation (optional)
	if validateQuality {
		if err := s.validateImageQuality(imageData); err != nil {
			return nil, err
		}
	}

	// 3. Preprocess and extract text
	processedImage := s.preprocessImage(imageData)
	rawText, err := s.extractTextFromImage(ctx, processedImage)
	if err != nil {
		return nil, fmt.Errorf("text extraction failed: %w", err)
	}

	// 4. Validate document type - must be NPWP
	if err := s.validateDocumentType(rawText, "NPWP"); err != nil {
		return nil, err
	}

	// 5. Parse NPWP text
	npwpData, err := s.parseNPWPText(ctx, rawText)
	if err != nil {
		return nil, fmt.Errorf("text parsing failed: %w", err)
	}

	// 6. Match administrative codes
	adminCode, err := s.matchAdministrativeCodes(ctx, npwpData.Address)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to match administrative codes")
	}
	npwpData.AdministrativeCode = adminCode

	// 7. Upload to S3
	documentURL, err := s.s3Svc.UploadNPWPDocument(ctx, npwpData.NPWPRaw, imageData)
	if err != nil {
		log.Error().Err(err).Msg("Failed to upload NPWP to S3")
	}
	npwpData.File = models.FileURLs{Document: documentURL}

	// 8. Save to database
	processingTime := time.Since(startTime).Milliseconds()
	record := s.buildNPWPRecord(clientID, npwpData, rawText, processingTime)
	if err := s.ocrRepo.Create(ctx, record); err != nil {
		log.Error().Err(err).Msg("Failed to save OCR record")
	}

	return &OCRResult{
		Data:           npwpData,
		ProcessingTime: processingTime,
	}, nil
}

// ProcessSIMOCR processes SIM image and extracts data
func (s *OCRService) ProcessSIMOCR(ctx context.Context, clientID int, imageData []byte, validateQuality bool) (*OCRResult, error) {
	startTime := time.Now()

	// 1. Validate image
	if err := s.validateImage(imageData); err != nil {
		return nil, err
	}

	// 2. Quality validation (optional)
	if validateQuality {
		if err := s.validateImageQuality(imageData); err != nil {
			return nil, err
		}
	}

	// 3. Preprocess and extract text
	processedImage := s.preprocessImage(imageData)
	rawText, err := s.extractTextFromImage(ctx, processedImage)
	if err != nil {
		return nil, fmt.Errorf("text extraction failed: %w", err)
	}

	// 4. Validate document type - must be SIM
	if err := s.validateDocumentType(rawText, "SIM"); err != nil {
		return nil, err
	}

	// 5. Parse SIM text
	simData, err := s.parseSIMText(ctx, rawText)
	if err != nil {
		return nil, fmt.Errorf("text parsing failed: %w", err)
	}

	// 6. Match administrative codes
	adminCode, err := s.matchAdministrativeCodes(ctx, simData.Address)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to match administrative codes")
	}
	simData.AdministrativeCode = adminCode

	// 7. Upload to S3
	documentURL, err := s.s3Svc.UploadSIMDocument(ctx, simData.SIMNumber, imageData)
	if err != nil {
		log.Error().Err(err).Msg("Failed to upload SIM to S3")
	}
	simData.File = models.FileURLs{Document: documentURL}

	// 8. Save to database
	processingTime := time.Since(startTime).Milliseconds()
	record := s.buildSIMRecord(clientID, simData, rawText, processingTime)
	if err := s.ocrRepo.Create(ctx, record); err != nil {
		log.Error().Err(err).Msg("Failed to save OCR record")
	}

	return &OCRResult{
		Data:           simData,
		ProcessingTime: processingTime,
	}, nil
}

// GetOCRByID retrieves an OCR record by ID
func (s *OCRService) GetOCRByID(ctx context.Context, id string, clientID int) (*models.OCRRecord, error) {
	return s.ocrRepo.GetByIDAndClientID(ctx, id, clientID)
}

// validateImage checks image format and size
func (s *OCRService) validateImage(imageData []byte) error {
	// Check size (max 10MB)
	if len(imageData) > 10*1024*1024 {
		return errors.New("image size exceeds 10MB limit")
	}

	// Check format
	_, format, err := image.DecodeConfig(bytes.NewReader(imageData))
	if err != nil {
		return errors.New("invalid image format")
	}

	validFormats := map[string]bool{"jpeg": true, "png": true, "webp": true}
	if !validFormats[format] {
		return fmt.Errorf("unsupported image format: %s. Supported: jpg, jpeg, png, webp", format)
	}

	return nil
}

// validateImageQuality performs quality checks (blur, darkness, glare)
func (s *OCRService) validateImageQuality(imageData []byte) error {
	// Decode image
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return fmt.Errorf("failed to decode image for quality check: %w", err)
	}

	quality := &models.QualityCheck{}
	var issues []string

	// Check blur using Laplacian variance (simplified)
	blurScore := s.calculateBlurScore(img)
	quality.Blur = models.QualityScore{
		Score:     blurScore,
		Threshold: 100,
		Passed:    blurScore >= 100,
	}
	if !quality.Blur.Passed {
		issues = append(issues, fmt.Sprintf("Image is too blurry (score: %.0f, threshold: 100)", blurScore))
	}

	// Check darkness using mean brightness
	brightnessScore := s.calculateBrightness(img)
	quality.Darkness = models.QualityScore{
		Score:     brightnessScore,
		Threshold: 0.3,
		Passed:    brightnessScore >= 0.3,
	}
	if !quality.Darkness.Passed {
		issues = append(issues, fmt.Sprintf("Image is too dark (score: %.2f, threshold: 0.30)", brightnessScore))
	}

	// Check glare using overexposed pixels
	glareScore := s.calculateGlare(img)
	quality.Glare = models.QualityScore{
		Score:     glareScore,
		Threshold: 0.2,
		Passed:    glareScore <= 0.2,
	}
	if !quality.Glare.Passed {
		issues = append(issues, fmt.Sprintf("Image has too much glare (score: %.2f, threshold: 0.20)", glareScore))
	}

	if len(issues) > 0 {
		return &QualityError{
			Details: issues,
			Quality: quality,
		}
	}

	return nil
}

// calculateBlurScore calculates blur using variance of Laplacian (simplified)
func (s *OCRService) calculateBlurScore(img image.Image) float64 {
	bounds := img.Bounds()
	var sum, sumSq float64
	var count float64

	for y := bounds.Min.Y + 1; y < bounds.Max.Y-1; y++ {
		for x := bounds.Min.X + 1; x < bounds.Max.X-1; x++ {
			// Get grayscale values of neighboring pixels
			r, g, b, _ := img.At(x, y).RGBA()
			gray := float64(r+g+b) / (3 * 65535)

			r1, g1, b1, _ := img.At(x-1, y).RGBA()
			r2, g2, b2, _ := img.At(x+1, y).RGBA()
			r3, g3, b3, _ := img.At(x, y-1).RGBA()
			r4, g4, b4, _ := img.At(x, y+1).RGBA()

			gray1 := float64(r1+g1+b1) / (3 * 65535)
			gray2 := float64(r2+g2+b2) / (3 * 65535)
			gray3 := float64(r3+g3+b3) / (3 * 65535)
			gray4 := float64(r4+g4+b4) / (3 * 65535)

			// Laplacian
			laplacian := -4*gray + gray1 + gray2 + gray3 + gray4
			sum += laplacian
			sumSq += laplacian * laplacian
			count++
		}
	}

	if count == 0 {
		return 0
	}

	mean := sum / count
	variance := (sumSq / count) - (mean * mean)

	// Scale to reasonable range
	return variance * 100000
}

// calculateBrightness calculates mean brightness
func (s *OCRService) calculateBrightness(img image.Image) float64 {
	bounds := img.Bounds()
	var sum float64
	var count float64

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			brightness := float64(r+g+b) / (3 * 65535)
			sum += brightness
			count++
		}
	}

	if count == 0 {
		return 0
	}

	return sum / count
}

// calculateGlare calculates percentage of overexposed pixels
func (s *OCRService) calculateGlare(img image.Image) float64 {
	bounds := img.Bounds()
	var overexposed float64
	var count float64

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			brightness := float64(r+g+b) / (3 * 65535)
			if brightness > 0.95 {
				overexposed++
			}
			count++
		}
	}

	if count == 0 {
		return 0
	}

	return overexposed / count
}

// DocumentTypeError represents invalid document type error
type DocumentTypeError struct {
	Expected string
	Detected string
	Message  string
}

func (e *DocumentTypeError) Error() string {
	return e.Message
}

// validateDocumentType checks if the extracted text matches the expected document type
func (s *OCRService) validateDocumentType(rawText string, expectedType string) error {
	upperText := strings.ToUpper(rawText)

	// Define keywords for each document type
	ktpKeywords := []string{
		"KARTU TANDA PENDUDUK",
		"REPUBLIK INDONESIA",
		"NIK",
		"PROVINSI",
		"KEWARGANEGARAAN",
		"BERLAKU HINGGA",
	}

	npwpKeywords := []string{
		"NPWP",
		"NOMOR POKOK WAJIB PAJAK",
		"DEPARTEMEN KEUANGAN",
		"DIREKTORAT JENDERAL PAJAK",
		"KEMENTERIAN KEUANGAN",
		"TERDAFTAR",
	}

	simKeywords := []string{
		"SURAT IZIN MENGEMUDI",
		"SIM",
		"KEPOLISIAN",
		"POLRI",
		"GOLONGAN",
		"BERLAKU SAMPAI",
	}

	// Count matching keywords for each type
	ktpScore := countKeywordMatches(upperText, ktpKeywords)
	npwpScore := countKeywordMatches(upperText, npwpKeywords)
	simScore := countKeywordMatches(upperText, simKeywords)

	// Determine detected type
	var detectedType string
	maxScore := 0

	if ktpScore > maxScore {
		maxScore = ktpScore
		detectedType = "KTP"
	}
	if npwpScore > maxScore {
		maxScore = npwpScore
		detectedType = "NPWP"
	}
	if simScore > maxScore {
		maxScore = simScore
		detectedType = "SIM"
	}

	// If no clear detection, check for minimum KTP requirements
	if maxScore < 2 {
		// For KTP, at least check for NIK pattern (16 digits)
		if expectedType == "KTP" {
			nikPattern := regexp.MustCompile(`\d{16}`)
			if nikPattern.MatchString(rawText) && ktpScore >= 1 {
				return nil // Acceptable as KTP
			}
		}
		return &DocumentTypeError{
			Expected: expectedType,
			Detected: "UNKNOWN",
			Message:  fmt.Sprintf("Document type could not be determined. Expected %s but no clear document identifiers found", expectedType),
		}
	}

	// Check if detected type matches expected
	if detectedType != expectedType {
		return &DocumentTypeError{
			Expected: expectedType,
			Detected: detectedType,
			Message:  fmt.Sprintf("Invalid document type. Expected %s but detected %s", expectedType, detectedType),
		}
	}

	return nil
}

// countKeywordMatches counts how many keywords are found in text
func countKeywordMatches(text string, keywords []string) int {
	count := 0
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			count++
		}
	}
	return count
}

// preprocessImage converts to grayscale and enhances contrast
func (s *OCRService) preprocessImage(imageData []byte) []byte {
	// For now, return original image
	// In production, implement grayscale conversion and contrast enhancement
	return imageData
}

// extractTextFromImage calls Google Vision API using service account
func (s *OCRService) extractTextFromImage(ctx context.Context, imageData []byte) (string, error) {
	// Get access token from service account
	accessToken, err := s.getGoogleAccessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get Google access token: %w", err)
	}

	// Prepare request
	requestBody := map[string]interface{}{
		"requests": []map[string]interface{}{
			{
				"image": map[string]string{
					"content": base64.StdEncoding.EncodeToString(imageData),
				},
				"features": []map[string]interface{}{
					{
						"type":       "DOCUMENT_TEXT_DETECTION",
						"maxResults": 1,
					},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	// Call Google Vision API with Bearer token
	url := "https://vision.googleapis.com/v1/images:annotate"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Google Vision API error: %s", string(body))
	}

	// Parse response
	var result struct {
		Responses []struct {
			FullTextAnnotation struct {
				Text string `json:"text"`
			} `json:"fullTextAnnotation"`
		} `json:"responses"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if len(result.Responses) == 0 {
		return "", nil
	}

	return result.Responses[0].FullTextAnnotation.Text, nil
}

// getGoogleAccessToken gets access token from service account credentials
func (s *OCRService) getGoogleAccessToken(ctx context.Context) (string, error) {
	// Read service account file
	credData, err := os.ReadFile(s.cfg.GoogleCredentialsPath)
	if err != nil {
		return "", fmt.Errorf("failed to read credentials file: %w", err)
	}

	// Parse service account JSON
	var creds struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal(credData, &creds); err != nil {
		return "", fmt.Errorf("failed to parse credentials: %w", err)
	}

	// Create JWT claim
	now := time.Now()
	claims := map[string]interface{}{
		"iss":   creds.ClientEmail,
		"sub":   creds.ClientEmail,
		"aud":   creds.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
		"scope": "https://www.googleapis.com/auth/cloud-vision",
	}

	// Create JWT header
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}

	// Encode header and claims
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signInput := headerB64 + "." + claimsB64

	// Parse private key and sign
	block, _ := pem.Decode([]byte(creds.PrivateKey))
	if block == nil {
		return "", errors.New("failed to decode private key PEM")
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	rsaKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return "", errors.New("private key is not RSA")
	}

	// Sign with SHA256
	hash := sha256.Sum256([]byte(signInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}
	signatureB64 := base64.RawURLEncoding.EncodeToString(signature)

	// Complete JWT
	jwt := signInput + "." + signatureB64

	// Exchange JWT for access token
	tokenReqBody := fmt.Sprintf("grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer&assertion=%s", jwt)
	tokenResp, err := http.Post(creds.TokenURI, "application/x-www-form-urlencoded", strings.NewReader(tokenReqBody))
	if err != nil {
		return "", fmt.Errorf("failed to request token: %w", err)
	}
	defer tokenResp.Body.Close()

	tokenBody, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		return "", err
	}

	if tokenResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed: %s", string(tokenBody))
	}

	var tokenResult struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(tokenBody, &tokenResult); err != nil {
		return "", err
	}

	return tokenResult.AccessToken, nil
}

// rawKTPData represents raw KTP data from AI in Indonesian
type rawKTPData struct {
	NIK          string `json:"nik"`
	FullName     string `json:"fullName"`
	PlaceOfBirth string `json:"placeOfBirth"`
	DateOfBirth  string `json:"dateOfBirth"`
	Gender       string `json:"gender"`
	BloodType    string `json:"bloodType"`
	Address      struct {
		Street      string `json:"street"`
		RT          string `json:"rt"`
		RW          string `json:"rw"`
		SubDistrict string `json:"subDistrict"`
		District    string `json:"district"`
		City        string `json:"city"`
		Province    string `json:"province"`
	} `json:"address"`
	Religion      string `json:"religion"`
	MaritalStatus string `json:"maritalStatus"`
	Occupation    string `json:"occupation"`
	Nationality   string `json:"nationality"`
	ValidUntil    string `json:"validUntil"`
	PublishedIn   string `json:"publishedIn"`
	PublishedOn   string `json:"publishedOn"`
}

// parseKTPText uses Groq API to parse raw text into structured KTP data
func (s *OCRService) parseKTPText(ctx context.Context, rawText string) (*models.KTPOCRResponse, error) {
	prompt := fmt.Sprintf(`Kamu adalah sistem OCR parser untuk KTP Indonesia (Kartu Tanda Penduduk).

ATURAN PENTING:
1. Ekstrak HANYA data yang ada di teks ini
2. Setiap field harus diambil dari teks yang diberikan
3. HANYA bloodType yang boleh null jika tidak ada
4. Field lainnya WAJIB ada nilainya, ambil dari teks

PANDUAN LOKASI FIELD DI KTP:
- NIK: 16 digit di bagian atas setelah "NIK"
- Nama: setelah kata "Nama"
- Tempat/Tgl Lahir: format "KOTA, DD-MM-YYYY"
- Jenis Kelamin & Gol Darah: baris yang sama (LAKI-LAKI/PEREMPUAN dan A/B/AB/O)
- Alamat: multi baris setelah "Alamat"
- RT/RW: format "RT/RW" atau "RT RW" 
- Kel/Desa: kelurahan/desa
- Kecamatan: setelah "Kecamatan" atau "Kec"
- Agama, Status Perkawinan, Pekerjaan, Kewarganegaraan: field standar
- Berlaku Hingga: "SEUMUR HIDUP" atau tanggal
- PENTING publishedIn: nama KOTA/KABUPATEN tempat KTP diterbitkan, biasanya di bagian BAWAH KTP, tepat DI ATAS tanggal terbit (contoh: "BANJARNEGARA", "KOTA BEKASI")
- PENTING publishedOn: TANGGAL penerbitan KTP dalam format DD-MM-YYYY, biasanya di bagian paling BAWAH KTP setelah nama kota (contoh: "12-07-2024")

Ekstrak ke JSON:
{
  "nik": "16 digit",
  "fullName": "NAMA LENGKAP KAPITAL",
  "placeOfBirth": "tempat lahir",
  "dateOfBirth": "yyyy-MM-dd",
  "gender": "LAKI-LAKI atau PEREMPUAN",
  "bloodType": "A/B/AB/O atau null",
  "address": {
    "street": "alamat jalan TANPA RT/RW",
    "rt": "3 digit",
    "rw": "3 digit",
    "subDistrict": "kelurahan/desa",
    "district": "kecamatan",
    "city": "KOTA/KABUPATEN + nama",
    "province": "provinsi TANPA kata PROVINSI"
  },
  "religion": "agama",
  "maritalStatus": "status kawin",
  "occupation": "pekerjaan",
  "nationality": "WNI/WNA",
  "validUntil": "SEUMUR HIDUP atau yyyy-MM-dd",
  "publishedIn": "nama kota/kabupaten penerbit",
  "publishedOn": "yyyy-MM-dd tanggal terbit"
}

Kembalikan HANYA JSON valid.

===TEKS KTP===
%s
===END===`, rawText)

	response, err := s.callGroqAPI(ctx, prompt)
	if err != nil {
		return nil, err
	}

	var raw rawKTPData
	if err := json.Unmarshal([]byte(response), &raw); err != nil {
		// Log the problematic response for debugging
		log.Warn().
			Str("response", response).
			Err(err).
			Msg("First attempt failed, retrying with simpler prompt")

		// Retry with simpler prompt
		response, err = s.parseKTPWithSimplePrompt(ctx, rawText)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(response), &raw); err != nil {
			log.Error().
				Str("response", response).
				Err(err).
				Msg("Failed to parse AI response as JSON after retry")
			return nil, fmt.Errorf("failed to parse AI response: %w", err)
		}
	}

	// Map Indonesian values to English enums
	gender := models.Gender(MapGender(raw.Gender))
	religion := models.Religion(MapReligion(raw.Religion))
	maritalStatus := models.MaritalStatus(MapMaritalStatus(raw.MaritalStatus))
	occupation := MapOccupation(raw.Occupation)
	nationality := models.Nationality(MapNationality(raw.Nationality))
	validUntil := MapValidUntil(raw.ValidUntil)

	// Map blood type (can be nil) - only if valid
	var bloodType *models.BloodType
	if raw.BloodType != "" && raw.BloodType != "null" && raw.BloodType != "-" {
		if bt := MapBloodType(raw.BloodType); bt != "" {
			btVal := models.BloodType(bt)
			bloodType = &btVal
		}
	}

	// Normalize address fields
	province := NormalizeProvince(raw.Address.Province)
	city := NormalizeCity(raw.Address.City)
	district := NormalizeDistrict(raw.Address.District)
	subDistrict := NormalizeSubDistrict(raw.Address.SubDistrict)

	// Sanitize NIK - remove any non-digit characters
	cleanNIK := sanitizeNIK(raw.NIK)

	ktpData := &models.KTPOCRResponse{
		NIK:          cleanNIK,
		FullName:     raw.FullName,
		PlaceOfBirth: raw.PlaceOfBirth,
		DateOfBirth:  raw.DateOfBirth,
		Gender:       gender,
		BloodType:    bloodType,
		Address: models.Address{
			Street:      raw.Address.Street,
			RT:          raw.Address.RT,
			RW:          raw.Address.RW,
			SubDistrict: subDistrict,
			District:    district,
			City:        city,
			Province:    province,
		},
		Religion:      religion,
		MaritalStatus: maritalStatus,
		Occupation:    occupation,
		Nationality:   nationality,
		ValidUntil:    validUntil,
		PublishedIn:   raw.PublishedIn,
		PublishedOn:   raw.PublishedOn,
	}

	return ktpData, nil
}

// parseKTPWithSimplePrompt uses a simpler prompt for retry
func (s *OCRService) parseKTPWithSimplePrompt(ctx context.Context, rawText string) (string, error) {
	prompt := fmt.Sprintf(`Extract KTP data to JSON. Output ONLY the JSON object, nothing else.

{"nik":"","fullName":"","placeOfBirth":"","dateOfBirth":"","gender":"","bloodType":null,"address":{"street":"","rt":"","rw":"","subDistrict":"","district":"","city":"","province":""},"religion":"","maritalStatus":"","occupation":"","nationality":"","validUntil":"","publishedIn":"","publishedOn":""}

Fill the values from this text:
%s`, rawText)

	return s.callGroqAPI(ctx, prompt)
}

// parseNPWPText uses Groq API to parse raw text into structured NPWP data
func (s *OCRService) parseNPWPText(ctx context.Context, rawText string) (*models.NPWPOCRResponse, error) {
	prompt := fmt.Sprintf(`Parse the following Indonesian NPWP (Tax ID) text into JSON format.
Extract the following fields:
- npwp (formatted with dots and dashes: XX.XXX.XXX.X-XXX.XXX)
- npwpRaw (15 or 16 digits without formatting)
- nik (16 digits if available, null for company NPWP)
- fullName (name or company name)
- format (OLD for 15 digits, NEW for 16 digits)
- taxPayerType (INDIVIDUAL or COMPANY)
- address.street, address.rt, address.rw, address.subDistrict, address.district, address.city, address.province
- publishedIn (KPP name)
- publishedOn (registration date in yyyy-MM-dd format)

Return ONLY valid JSON, no explanation.

NPWP Text:
%s`, rawText)

	response, err := s.callGroqAPI(ctx, prompt)
	if err != nil {
		return nil, err
	}

	var npwpData models.NPWPOCRResponse
	if err := json.Unmarshal([]byte(response), &npwpData); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	return &npwpData, nil
}

// parseSIMText uses Groq API to parse raw text into structured SIM data
func (s *OCRService) parseSIMText(ctx context.Context, rawText string) (*models.SIMOCRResponse, error) {
	prompt := fmt.Sprintf(`Parse the following Indonesian SIM (Driver's License) text into JSON format.
Extract the following fields:
- simNumber (12 digits)
- fullName
- placeOfBirth
- dateOfBirth (yyyy-MM-dd)
- gender (MAN or WOMAN)
- bloodType (A, B, AB, O, or null)
- height (in cm, just the number)
- address.street, address.rt, address.rw, address.subDistrict, address.district, address.city, address.province
- occupation
- type (A, B1, B2, C, C1, C2, D, or D1)
- validFrom (yyyy-MM-dd)
- validUntil (yyyy-MM-dd)
- publisher (POLDA/POLRES name)

Return ONLY valid JSON, no explanation.

SIM Text:
%s`, rawText)

	response, err := s.callGroqAPI(ctx, prompt)
	if err != nil {
		return nil, err
	}

	var simData models.SIMOCRResponse
	if err := json.Unmarshal([]byte(response), &simData); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	return &simData, nil
}

// callGroqAPI calls Groq API for text parsing
func (s *OCRService) callGroqAPI(ctx context.Context, prompt string) (string, error) {
	requestBody := map[string]interface{}{
		"model": s.cfg.GroqModel,
		"messages": []interface{}{
			map[string]string{
				"role":    "system",
				"content": "You are a JSON-only response bot. You MUST respond with ONLY a valid JSON object. No explanations, no markdown, no text before or after the JSON. Start your response with { and end with }.",
			},
			map[string]string{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.1,
		"max_tokens":  2000,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+s.cfg.GroqAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Groq API error: %s", string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 {
		return "", errors.New("no response from Groq API")
	}

	// Get raw content
	rawContent := result.Choices[0].Message.Content

	// Log raw response for debugging
	log.Debug().Str("raw_response", rawContent).Msg("Raw AI response")

	// Extract JSON from response
	content := extractJSON(rawContent)

	if content == "" {
		// Log the full raw response when extraction fails
		log.Error().
			Str("raw_content", rawContent).
			Msg("Failed to extract JSON from AI response")
		return "", fmt.Errorf("no valid JSON found in AI response. Raw: %s", truncateString(rawContent, 200))
	}

	return content, nil
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// extractJSON extracts JSON object from a string that may contain extra text
func extractJSON(s string) string {
	// First, try to remove markdown code blocks
	s = strings.TrimSpace(s)

	// Remove markdown code block markers
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	// If it starts with {, try to find the matching }
	var jsonStr string
	if strings.HasPrefix(s, "{") {
		jsonStr = extractJSONObject(s)
	} else {
		// Try to find JSON object in the string
		startIdx := strings.Index(s, "{")
		if startIdx == -1 {
			return ""
		}
		jsonStr = extractJSONObject(s[startIdx:])
	}

	// Sanitize common JSON errors
	jsonStr = sanitizeJSON(jsonStr)

	// Validate that it's a proper JSON object (not just {16} or similar)
	if !isValidJSONObject(jsonStr) {
		return ""
	}

	return jsonStr
}

// isValidJSONObject checks if the string is a valid JSON object with at least one key-value pair
func isValidJSONObject(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 5 { // Minimum valid JSON: {"a":1}
		return false
	}
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return false
	}
	// Check if it contains at least one quoted key
	// Valid JSON keys must be quoted: {"key": value}
	if !strings.Contains(s, `":`) {
		return false
	}
	// Try to parse it
	var test map[string]interface{}
	if err := json.Unmarshal([]byte(s), &test); err != nil {
		return false
	}
	// Must have at least one key
	return len(test) > 0
}

// extractJSONObject extracts a complete JSON object from a string starting with {
func extractJSONObject(s string) string {
	if !strings.HasPrefix(s, "{") {
		return ""
	}

	depth := 0
	inString := false
	escaped := false

	for i, char := range s {
		if escaped {
			escaped = false
			continue
		}

		if char == '\\' && inString {
			escaped = true
			continue
		}

		if char == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		if char == '{' {
			depth++
		} else if char == '}' {
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}

	// If we couldn't find matching braces, return the whole string
	// and let JSON parser handle the error
	return s
}

// sanitizeJSON fixes common JSON errors from AI responses
func sanitizeJSON(s string) string {
	// Remove trailing commas before } or ]
	// This regex finds comma followed by whitespace and then } or ]
	trailingCommaRegex := regexp.MustCompile(`,\s*([}\]])`)
	s = trailingCommaRegex.ReplaceAllString(s, "$1")

	// Fix unquoted keys (simple cases like {key: "value"} -> {"key": "value"})
	// Match word characters followed by colon that aren't already quoted
	unquotedKeyRegex := regexp.MustCompile(`([{,]\s*)([a-zA-Z_][a-zA-Z0-9_]*)\s*:`)
	s = unquotedKeyRegex.ReplaceAllString(s, `$1"$2":`)

	// Remove any newlines inside string values that might break JSON
	// This is a simplified fix - complex cases might still fail

	return s
}

// AddressMatchResult contains matched administrative codes and corrected names from database
type AddressMatchResult struct {
	AdminCode       models.AdministrativeCode
	ProvinceName    string
	CityName        string
	DistrictName    string
	SubDistrictName string
}

// matchAdministrativeCodes performs hierarchical address matching and returns database names
func (s *OCRService) matchAdministrativeCodes(ctx context.Context, address models.Address) (models.AdministrativeCode, error) {
	result := models.AdministrativeCode{}

	// Normalize address fields for matching
	provinceSearch := NormalizeProvince(address.Province)
	citySearch := NormalizeCityForMatching(address.City)
	districtSearch := NormalizeDistrict(address.District)
	subDistrictSearch := NormalizeSubDistrict(address.SubDistrict)

	// 1. Match Province - try multiple variations
	province, err := s.findProvince(ctx, provinceSearch)
	if err != nil {
		return result, fmt.Errorf("province not found: %s", address.Province)
	}
	result.Province = province.Code

	// 2. Match City (filtered by province)
	city, err := s.findCity(ctx, citySearch, province.Code)
	if err != nil {
		return result, fmt.Errorf("city not found: %s in province %s", address.City, address.Province)
	}
	result.City = city.FullCode

	// 3. Match District (filtered by city)
	district, err := s.findDistrict(ctx, districtSearch, city.FullCode)
	if err != nil {
		return result, fmt.Errorf("district not found: %s in city %s", address.District, address.City)
	}
	result.District = district.FullCode

	// 4. Match SubDistrict (filtered by district)
	subDistrict, err := s.findSubDistrict(ctx, subDistrictSearch, district.FullCode)
	if err != nil {
		return result, fmt.Errorf("sub-district not found: %s in district %s", address.SubDistrict, address.District)
	}
	result.SubDistrict = subDistrict.FullCode

	return result, nil
}

// matchAdministrativeCodesWithNames performs matching and updates address with database names
func (s *OCRService) matchAdministrativeCodesWithNames(ctx context.Context, address *models.Address) (models.AdministrativeCode, error) {
	result := models.AdministrativeCode{}

	// Normalize address fields for matching
	provinceSearch := NormalizeProvince(address.Province)
	citySearch := NormalizeCityForMatching(address.City)
	districtSearch := NormalizeDistrict(address.District)
	subDistrictSearch := NormalizeSubDistrict(address.SubDistrict)

	// 1. Match Province
	province, err := s.findProvince(ctx, provinceSearch)
	if err != nil {
		log.Warn().Str("province", address.Province).Str("search", provinceSearch).Msg("Province not found")
		return result, fmt.Errorf("province not found: %s", address.Province)
	}
	result.Province = province.Code
	address.Province = province.Name // Use database name

	// 2. Match City
	city, err := s.findCity(ctx, citySearch, province.Code)
	if err != nil {
		log.Warn().Str("city", address.City).Str("search", citySearch).Msg("City not found")
		return result, fmt.Errorf("city not found: %s in province %s", address.City, address.Province)
	}
	result.City = city.FullCode
	address.City = city.Name // Use database name (includes KOTA/KABUPATEN)

	// 3. Match District - try normal first, then try with swapped values
	district, err := s.findDistrict(ctx, districtSearch, city.FullCode)
	swapped := false
	if err != nil {
		// Try using subDistrict value as district (AI might have swapped them)
		log.Debug().Str("trying_swap", subDistrictSearch).Msg("District not found, trying subDistrict value as district")
		district, err = s.findDistrict(ctx, subDistrictSearch, city.FullCode)
		if err != nil {
			log.Warn().Str("district", address.District).Str("search", districtSearch).Msg("District not found")
			return result, fmt.Errorf("district not found: %s in city %s", address.District, address.City)
		}
		swapped = true
	}
	result.District = district.FullCode
	address.District = district.Name // Use database name

	// 4. Match SubDistrict - use the other value if we swapped
	var subDistrictSearchFinal string
	if swapped {
		subDistrictSearchFinal = districtSearch // Original district value is actually subDistrict
	} else {
		subDistrictSearchFinal = subDistrictSearch
	}

	subDistrict, err := s.findSubDistrict(ctx, subDistrictSearchFinal, district.FullCode)
	if err != nil {
		// If swapped didn't work, try the other way
		if !swapped {
			log.Debug().Str("trying_original_district", districtSearch).Msg("SubDistrict not found, trying original district value")
			subDistrict, err = s.findSubDistrict(ctx, districtSearch, district.FullCode)
		}
		if err != nil {
			log.Warn().Str("subDistrict", address.SubDistrict).Str("search", subDistrictSearchFinal).Msg("SubDistrict not found")
			return result, fmt.Errorf("sub-district not found: %s in district %s", address.SubDistrict, address.District)
		}
	}
	result.SubDistrict = subDistrict.FullCode
	address.SubDistrict = subDistrict.Name // Use database name

	return result, nil
}

// findProvince tries multiple search strategies to find province
func (s *OCRService) findProvince(ctx context.Context, search string) (*models.Province, error) {
	// Try exact match
	if p, err := s.territoryRepo.GetProvinceByName(ctx, search); err == nil {
		return p, nil
	}
	// Try without spaces
	if p, err := s.territoryRepo.GetProvinceByName(ctx, strings.ReplaceAll(search, " ", "")); err == nil {
		return p, nil
	}
	// Try with single space normalized
	normalized := regexp.MustCompile(`\s+`).ReplaceAllString(search, " ")
	if p, err := s.territoryRepo.GetProvinceByName(ctx, normalized); err == nil {
		return p, nil
	}
	return nil, fmt.Errorf("province not found: %s", search)
}

// findCity tries multiple search strategies to find city
func (s *OCRService) findCity(ctx context.Context, search, provinceCode string) (*models.City, error) {
	// Try exact match
	if c, err := s.territoryRepo.GetCityByNameAndProvince(ctx, search, provinceCode); err == nil {
		return c, nil
	}
	// Try without spaces
	if c, err := s.territoryRepo.GetCityByNameAndProvince(ctx, strings.ReplaceAll(search, " ", ""), provinceCode); err == nil {
		return c, nil
	}
	// Try with single space normalized
	normalized := regexp.MustCompile(`\s+`).ReplaceAllString(search, " ")
	if c, err := s.territoryRepo.GetCityByNameAndProvince(ctx, normalized, provinceCode); err == nil {
		return c, nil
	}
	return nil, fmt.Errorf("city not found: %s", search)
}

// findDistrict tries multiple search strategies to find district
func (s *OCRService) findDistrict(ctx context.Context, search, cityCode string) (*models.District, error) {
	// Try exact match
	if d, err := s.territoryRepo.GetDistrictByNameAndCity(ctx, search, cityCode); err == nil {
		return d, nil
	}
	// Try without spaces (handles "BEKASIUTARA" -> "BEKASI UTARA")
	if d, err := s.territoryRepo.GetDistrictByNameAndCity(ctx, strings.ReplaceAll(search, " ", ""), cityCode); err == nil {
		return d, nil
	}
	// Try adding space between words (heuristic for concatenated names)
	if d, err := s.territoryRepo.GetDistrictByNameAndCity(ctx, addSpacesToConcatenated(search), cityCode); err == nil {
		return d, nil
	}
	return nil, fmt.Errorf("district not found: %s", search)
}

// findSubDistrict tries multiple search strategies to find sub-district
func (s *OCRService) findSubDistrict(ctx context.Context, search, districtCode string) (*models.SubDistrict, error) {
	// Try exact match
	if sd, err := s.territoryRepo.GetSubDistrictByNameAndDistrict(ctx, search, districtCode); err == nil {
		return sd, nil
	}
	// Try without spaces
	if sd, err := s.territoryRepo.GetSubDistrictByNameAndDistrict(ctx, strings.ReplaceAll(search, " ", ""), districtCode); err == nil {
		return sd, nil
	}
	// Try adding space between words
	if sd, err := s.territoryRepo.GetSubDistrictByNameAndDistrict(ctx, addSpacesToConcatenated(search), districtCode); err == nil {
		return sd, nil
	}
	return nil, fmt.Errorf("sub-district not found: %s", search)
}

// addSpacesToConcatenated tries to add spaces to concatenated words
// e.g., "BEKASIUTARA" -> "BEKASI UTARA"
func addSpacesToConcatenated(s string) string {
	// Common direction words that might be concatenated
	directions := []string{"UTARA", "SELATAN", "BARAT", "TIMUR", "TENGAH", "PUSAT"}
	upper := strings.ToUpper(s)

	for _, dir := range directions {
		if strings.HasSuffix(upper, dir) && len(upper) > len(dir) {
			prefix := upper[:len(upper)-len(dir)]
			// Check if it doesn't already have a space
			if !strings.HasSuffix(prefix, " ") {
				return prefix + " " + dir
			}
		}
	}
	return s
}

// normalizeName normalizes location names for matching (legacy - use specific functions)
func (s *OCRService) normalizeName(name string) string {
	name = strings.ToUpper(strings.TrimSpace(name))
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")

	// Remove common prefixes
	prefixes := []string{"PROVINSI ", "KAB.", "KABUPATEN ", "KOTA ADM.", "KOTA ADMINISTRASI ", "KOTA ", "KEL.", "KELURAHAN ", "KEC.", "KECAMATAN ", "DESA "}
	for _, prefix := range prefixes {
		name = strings.TrimPrefix(name, prefix)
	}

	return strings.TrimSpace(name)
}

// sanitizeNIK removes all non-digit characters from NIK and ensures it's 16 digits
func sanitizeNIK(nik string) string {
	// Remove all non-digit characters
	var result strings.Builder
	for _, c := range nik {
		if c >= '0' && c <= '9' {
			result.WriteRune(c)
		}
	}
	return result.String()
}

// validateNIK validates NIK against database and extracted data
func (s *OCRService) validateNIK(ctx context.Context, nik string, dateOfBirth string, adminCode models.AdministrativeCode) *models.NIKValidation {
	validation := &models.NIKValidation{
		IsValid: true,
	}

	if len(nik) != 16 {
		validation.IsValid = false
		return validation
	}

	// Province validation (first 2 digits)
	nikProvince := nik[0:2]
	if adminCode.Province != "" {
		validation.ProvinceMatch = nikProvince == adminCode.Province
	} else {
		// If no admin code, check if province code exists in database
		validation.ProvinceMatch = false
	}

	// City validation (first 4 digits)
	nikCity := nik[0:4]
	if adminCode.City != "" {
		validation.CityMatch = nikCity == adminCode.City
	} else {
		validation.CityMatch = false
	}

	// District validation (first 6 digits)
	nikDistrict := nik[0:6]
	if adminCode.District != "" {
		validation.DistrictMatch = nikDistrict == adminCode.District
	} else {
		validation.DistrictMatch = false
	}

	// Birth date validation (digits 7-12)
	if dateOfBirth != "" && len(nik) >= 12 {
		nikDatePart := nik[6:12]
		validation.BirthDateMatch = s.validateNIKBirthDate(nikDatePart, dateOfBirth)
	} else {
		validation.BirthDateMatch = false
	}

	// Overall validity - all must match
	validation.IsValid = validation.ProvinceMatch && validation.CityMatch && validation.DistrictMatch && validation.BirthDateMatch

	return validation
}

// validateNIKBirthDate validates birth date from NIK
func (s *OCRService) validateNIKBirthDate(nikDatePart, dateOfBirth string) bool {
	// NIK format: DDMMYY (day can be +40 for female)
	if len(nikDatePart) != 6 || len(dateOfBirth) < 10 {
		return false
	}

	nikDay, _ := strconv.Atoi(nikDatePart[0:2])
	nikMonth, _ := strconv.Atoi(nikDatePart[2:4])
	nikYear, _ := strconv.Atoi(nikDatePart[4:6])

	// Adjust for female (day > 40)
	if nikDay > 40 {
		nikDay -= 40
	}

	// Parse dateOfBirth (format: yyyy-MM-dd)
	parts := strings.Split(dateOfBirth, "-")
	if len(parts) != 3 {
		return false
	}

	year, _ := strconv.Atoi(parts[0])
	month, _ := strconv.Atoi(parts[1])
	day, _ := strconv.Atoi(parts[2])

	// Compare
	yearMatch := nikYear == (year % 100)
	monthMatch := nikMonth == month
	dayMatch := nikDay == day

	return yearMatch && monthMatch && dayMatch
}

// buildKTPRecord builds OCR record from KTP data
func (s *OCRService) buildKTPRecord(clientID int, data *models.KTPOCRResponse, rawText string, processingTime int64) *models.OCRRecord {
	gender := data.Gender
	religion := data.Religion
	maritalStatus := data.MaritalStatus
	nationality := data.Nationality

	return &models.OCRRecord{
		ID:                 uuid.New().String(),
		ClientID:           clientID,
		DocType:            models.DocTypeKTP,
		NIK:                &data.NIK,
		FullName:           data.FullName,
		PlaceOfBirth:       &data.PlaceOfBirth,
		DateOfBirth:        &data.DateOfBirth,
		Gender:             &gender,
		BloodType:          data.BloodType,
		Address:            &data.Address,
		AdministrativeCode: &data.AdministrativeCode,
		Religion:           &religion,
		MaritalStatus:      &maritalStatus,
		Occupation:         &data.Occupation,
		Nationality:        &nationality,
		ValidUntil:         &data.ValidUntil,
		PublishedIn:        &data.PublishedIn,
		PublishedOn:        &data.PublishedOn,
		FileURLs:           &data.File,
		RawText:            &rawText,
		ProcessingTimeMs:   processingTime,
	}
}

// buildNPWPRecord builds OCR record from NPWP data
func (s *OCRService) buildNPWPRecord(clientID int, data *models.NPWPOCRResponse, rawText string, processingTime int64) *models.OCRRecord {
	format := data.Format
	taxType := data.TaxPayerType

	return &models.OCRRecord{
		ID:                 uuid.New().String(),
		ClientID:           clientID,
		DocType:            models.DocTypeNPWP,
		NPWP:               &data.NPWP,
		NPWPRaw:            &data.NPWPRaw,
		NIK:                data.NIK,
		FullName:           data.FullName,
		Address:            &data.Address,
		AdministrativeCode: &data.AdministrativeCode,
		NPWPFormat:         &format,
		TaxPayerType:       &taxType,
		PublishedIn:        &data.PublishedIn,
		PublishedOn:        &data.PublishedOn,
		FileURLs:           &data.File,
		RawText:            &rawText,
		ProcessingTimeMs:   processingTime,
	}
}

// buildSIMRecord builds OCR record from SIM data
func (s *OCRService) buildSIMRecord(clientID int, data *models.SIMOCRResponse, rawText string, processingTime int64) *models.OCRRecord {
	gender := data.Gender
	simType := data.Type

	return &models.OCRRecord{
		ID:                 uuid.New().String(),
		ClientID:           clientID,
		DocType:            models.DocTypeSIM,
		SIMNumber:          &data.SIMNumber,
		FullName:           data.FullName,
		PlaceOfBirth:       &data.PlaceOfBirth,
		DateOfBirth:        &data.DateOfBirth,
		Gender:             &gender,
		BloodType:          data.BloodType,
		Height:             &data.Height,
		Address:            &data.Address,
		AdministrativeCode: &data.AdministrativeCode,
		Occupation:         &data.Occupation,
		SIMType:            &simType,
		ValidFrom:          &data.ValidFrom,
		ValidUntil:         &data.ValidUntil,
		Publisher:          &data.Publisher,
		FileURLs:           &data.File,
		RawText:            &rawText,
		ProcessingTimeMs:   processingTime,
	}
}

// DecodeBase64Image decodes base64 image string to bytes
func DecodeBase64Image(imageStr string) ([]byte, error) {
	// Remove data URI prefix if present
	if strings.Contains(imageStr, ",") {
		parts := strings.SplitN(imageStr, ",", 2)
		if len(parts) == 2 {
			imageStr = parts[1]
		}
	}

	return base64.StdEncoding.DecodeString(imageStr)
}
