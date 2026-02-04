package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// DocType represents the document type for OCR
type DocType string

const (
	DocTypeKTP  DocType = "KTP"
	DocTypeNPWP DocType = "NPWP"
	DocTypeSIM  DocType = "SIM"
)

// Gender represents gender enum
type Gender string

const (
	GenderMan   Gender = "MAN"
	GenderWoman Gender = "WOMAN"
)

// BloodType represents blood type enum
type BloodType string

const (
	BloodTypeA  BloodType = "A"
	BloodTypeB  BloodType = "B"
	BloodTypeAB BloodType = "AB"
	BloodTypeO  BloodType = "O"
)

// Religion represents religion enum
type Religion string

const (
	ReligionIslam      Religion = "ISLAM"
	ReligionProtestant Religion = "PROTESTANT"
	ReligionCatholic   Religion = "CATHOLIC"
	ReligionHindu      Religion = "HINDU"
	ReligionBuddha     Religion = "BUDDHA"
	ReligionKonghucu   Religion = "KONGHUCU"
	ReligionBelief     Religion = "BELIEF"
)

// MaritalStatus represents marital status enum
type MaritalStatus string

const (
	MaritalStatusUnmarried           MaritalStatus = "UNMARRIED"
	MaritalStatusMarried             MaritalStatus = "MARRIED"
	MaritalStatusLifeDivorce         MaritalStatus = "LIFE_DIVORCE"
	MaritalStatusDeathDivorce        MaritalStatus = "DEATH_DIVORCE"
	MaritalStatusUnregisteredMarried MaritalStatus = "UNREGISTERED_MARRIED"
)

// Nationality represents nationality enum
type Nationality string

const (
	NationalityWNI Nationality = "WNI"
	NationalityWNA Nationality = "WNA"
)

// NPWPFormat represents NPWP format enum
type NPWPFormat string

const (
	NPWPFormatOld NPWPFormat = "OLD"
	NPWPFormatNew NPWPFormat = "NEW"
)

// TaxPayerType represents tax payer type enum
type TaxPayerType string

const (
	TaxPayerTypeIndividual TaxPayerType = "INDIVIDUAL"
	TaxPayerTypeCompany    TaxPayerType = "COMPANY"
)

// SIMType represents SIM type enum
type SIMType string

const (
	SIMTypeA  SIMType = "A"
	SIMTypeB1 SIMType = "B1"
	SIMTypeB2 SIMType = "B2"
	SIMTypeC  SIMType = "C"
	SIMTypeC1 SIMType = "C1"
	SIMTypeC2 SIMType = "C2"
	SIMTypeD  SIMType = "D"
	SIMTypeD1 SIMType = "D1"
)

// Address represents extracted address from document
type Address struct {
	Street      string `json:"street"`
	RT          string `json:"rt"`
	RW          string `json:"rw"`
	SubDistrict string `json:"subDistrict"`
	District    string `json:"district"`
	City        string `json:"city"`
	Province    string `json:"province"`
}

// Value implements driver.Valuer for database storage
func (a Address) Value() (driver.Value, error) {
	return json.Marshal(a)
}

// Scan implements sql.Scanner for database retrieval
func (a *Address) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to scan Address")
	}
	return json.Unmarshal(bytes, a)
}

// AdministrativeCode represents administrative area codes
type AdministrativeCode struct {
	Province    string `json:"province"`
	City        string `json:"city"`
	District    string `json:"district"`
	SubDistrict string `json:"subDistrict"`
}

// Value implements driver.Valuer for database storage
func (ac AdministrativeCode) Value() (driver.Value, error) {
	return json.Marshal(ac)
}

// Scan implements sql.Scanner for database retrieval
func (ac *AdministrativeCode) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to scan AdministrativeCode")
	}
	return json.Unmarshal(bytes, ac)
}

// FileURLs represents document file URLs
type FileURLs struct {
	Document string `json:"document"`
}

// Value implements driver.Valuer for database storage
func (f FileURLs) Value() (driver.Value, error) {
	return json.Marshal(f)
}

// Scan implements sql.Scanner for database retrieval
func (f *FileURLs) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to scan FileURLs")
	}
	return json.Unmarshal(bytes, f)
}

// NIKValidation represents NIK validation result
type NIKValidation struct {
	IsValid        bool `json:"isValid"`
	ProvinceMatch  bool `json:"provinceMatch"`
	CityMatch      bool `json:"cityMatch"`
	DistrictMatch  bool `json:"districtMatch"`
	BirthDateMatch bool `json:"birthDateMatch"`
}

// QualityScore represents image quality check result
type QualityScore struct {
	Passed    bool    `json:"passed"`
	Score     float64 `json:"score"`
	Threshold float64 `json:"threshold"`
}

// QualityCheck represents overall quality validation result
type QualityCheck struct {
	Blur     QualityScore `json:"blur"`
	Darkness QualityScore `json:"darkness"`
	Glare    QualityScore `json:"glare"`
}

// OCRRecord represents an OCR record in database
type OCRRecord struct {
	ID                 string              `db:"id" json:"id"`
	ClientID           int                 `db:"client_id" json:"-"`
	DocType            DocType             `db:"doc_type" json:"docType"`
	NIK                *string             `db:"nik" json:"nik,omitempty"`
	NPWP               *string             `db:"npwp" json:"npwp,omitempty"`
	NPWPRaw            *string             `db:"npwp_raw" json:"npwpRaw,omitempty"`
	SIMNumber          *string             `db:"sim_number" json:"simNumber,omitempty"`
	FullName           string              `db:"full_name" json:"fullName"`
	PlaceOfBirth       *string             `db:"place_of_birth" json:"placeOfBirth,omitempty"`
	DateOfBirth        *string             `db:"date_of_birth" json:"dateOfBirth,omitempty"`
	Gender             *Gender             `db:"gender" json:"gender,omitempty"`
	BloodType          *BloodType          `db:"blood_type" json:"bloodType,omitempty"`
	Address            *Address            `db:"address" json:"address,omitempty"`
	AdministrativeCode *AdministrativeCode `db:"administrative_code" json:"administrativeCode,omitempty"`
	Religion           *Religion           `db:"religion" json:"religion,omitempty"`
	MaritalStatus      *MaritalStatus      `db:"marital_status" json:"maritalStatus,omitempty"`
	Occupation         *string             `db:"occupation" json:"occupation,omitempty"`
	Nationality        *Nationality        `db:"nationality" json:"nationality,omitempty"`
	ValidUntil         *string             `db:"valid_until" json:"validUntil,omitempty"`
	ValidFrom          *string             `db:"valid_from" json:"validFrom,omitempty"`
	PublishedIn        *string             `db:"published_in" json:"publishedIn,omitempty"`
	PublishedOn        *string             `db:"published_on" json:"publishedOn,omitempty"`
	Publisher          *string             `db:"publisher" json:"publisher,omitempty"`
	NPWPFormat         *NPWPFormat         `db:"npwp_format" json:"format,omitempty"`
	TaxPayerType       *TaxPayerType       `db:"tax_payer_type" json:"taxPayerType,omitempty"`
	SIMType            *SIMType            `db:"sim_type" json:"type,omitempty"`
	Height             *string             `db:"height" json:"height,omitempty"`
	FileURLs           *FileURLs           `db:"file_urls" json:"file,omitempty"`
	RawText            *string             `db:"raw_text" json:"-"`
	ProcessingTimeMs   int64               `db:"processing_time_ms" json:"-"`
	CreatedAt          time.Time           `db:"created_at" json:"createdAt"`
}

// OCRRequest represents common OCR request fields
type OCRRequest struct {
	Image           string `json:"image" form:"image"`
	ValidateQuality bool   `json:"validateQuality" form:"validateQuality"`
	ValidateNik     bool   `json:"validateNik" form:"validateNik"`
}

// KTPOCRResponse represents KTP OCR API response data
type KTPOCRResponse struct {
	NIK                string             `json:"nik"`
	FullName           string             `json:"fullName"`
	PlaceOfBirth       string             `json:"placeOfBirth"`
	DateOfBirth        string             `json:"dateOfBirth"`
	Gender             Gender             `json:"gender"`
	BloodType          *BloodType         `json:"bloodType"`
	Address            Address            `json:"address"`
	AdministrativeCode AdministrativeCode `json:"administrativeCode"`
	Religion           Religion           `json:"religion"`
	MaritalStatus      MaritalStatus      `json:"maritalStatus"`
	Occupation         string             `json:"occupation"`
	Nationality        Nationality        `json:"nationality"`
	ValidUntil         string             `json:"validUntil"`
	PublishedIn        string             `json:"publishedIn"`
	PublishedOn        string             `json:"publishedOn"`
	File               FileURLs           `json:"file"`
}

// NPWPOCRResponse represents NPWP OCR API response data
type NPWPOCRResponse struct {
	NPWP               string             `json:"npwp"`
	NPWPRaw            string             `json:"npwpRaw"`
	NIK                *string            `json:"nik"`
	FullName           string             `json:"fullName"`
	Format             NPWPFormat         `json:"format"`
	TaxPayerType       TaxPayerType       `json:"taxPayerType"`
	Address            Address            `json:"address"`
	AdministrativeCode AdministrativeCode `json:"administrativeCode"`
	PublishedIn        string             `json:"publishedIn"`
	PublishedOn        string             `json:"publishedOn"`
	File               FileURLs           `json:"file"`
}

// SIMOCRResponse represents SIM OCR API response data
type SIMOCRResponse struct {
	SIMNumber          string             `json:"simNumber"`
	FullName           string             `json:"fullName"`
	PlaceOfBirth       string             `json:"placeOfBirth"`
	DateOfBirth        string             `json:"dateOfBirth"`
	Gender             Gender             `json:"gender"`
	BloodType          *BloodType         `json:"bloodType"`
	Height             string             `json:"height"`
	Address            Address            `json:"address"`
	AdministrativeCode AdministrativeCode `json:"administrativeCode"`
	Occupation         string             `json:"occupation"`
	Type               SIMType            `json:"type"`
	ValidFrom          string             `json:"validFrom"`
	ValidUntil         string             `json:"validUntil"`
	Publisher          string             `json:"publisher"`
	File               FileURLs           `json:"file"`
}

// OCRMeta represents OCR response metadata
type OCRMeta struct {
	RequestID          string `json:"requestId"`
	Timestamp          string `json:"timestamp"`
	ProcessingTimeMs   int64  `json:"processingTimeMs"`
	ExtractionEngine   string `json:"extractionEngine"`
	VerificationEngine string `json:"verificationEngine"`
}

// OCRValidation represents OCR validation result in response
type OCRValidation struct {
	NIK *NIKValidation `json:"nik,omitempty"`
}
