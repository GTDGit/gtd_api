package kiosbank

import "encoding/json"

// BaseResponse is common response fields
type BaseResponse struct {
	RC          string `json:"rc"`
	Description string `json:"description,omitempty"`
	MerchantID  string `json:"merchantID,omitempty"`
	SessionID   string `json:"sessionID,omitempty"`
}

// SignOnResponse is the response from Sign On
type SignOnResponse struct {
	BaseResponse
}

// InquiryData contains inquiry result data
type InquiryData struct {
	IDPelanggan    string          `json:"idPelanggan"`
	Nama           string          `json:"nama"`
	TotalTagihan   string          `json:"totalTagihan,omitempty"`
	JumlahTagihan  string          `json:"jumlahTagihan,omitempty"`
	Admin          string          `json:"admin,omitempty"`
	Total          string          `json:"total,omitempty"`
	Period         string          `json:"periode,omitempty"`
	NoReferensi    string          `json:"noReferensi,omitempty"`
	Info           string          `json:"info,omitempty"`
	RincianTagihan json.RawMessage `json:"rincianTagihan,omitempty"`
}

// InquiryResponse is the response from Inquiry
type InquiryResponse struct {
	BaseResponse
	CustomerID  string      `json:"customerID"`
	ProductID   string      `json:"productID"`
	ReferenceID string      `json:"referenceID"`
	Data        InquiryData `json:"data"`
}

// PaymentData contains payment result data
type PaymentData struct {
	IDPelanggan  string          `json:"idPelanggan"`
	Nama         string          `json:"nama"`
	NoReferensi  string          `json:"noReferensi"`
	Tagihan      string          `json:"tagihan,omitempty"`
	Admin        string          `json:"admin,omitempty"`
	Total        string          `json:"total,omitempty"`
	Status       string          `json:"status,omitempty"`
	SerialNumber string          `json:"sn,omitempty"`
	Token        string          `json:"token,omitempty"`
	KWH          string          `json:"kwh,omitempty"`
	Info         json.RawMessage `json:"info,omitempty"`
}

// PaymentResponse is the response from Payment
type PaymentResponse struct {
	BaseResponse
	CustomerID  string      `json:"customerID"`
	ProductID   string      `json:"productID"`
	ReferenceID string      `json:"referenceID"`
	Data        PaymentData `json:"data"`
}

// SinglePaymentData contains single payment result data
type SinglePaymentData struct {
	IDPelanggan string `json:"idPelanggan"`
	Nama        string `json:"nama"`
	NoReferensi string `json:"noReferensi"`
	Harga       string `json:"harga"`
	Status      string `json:"status"`
}

// SinglePaymentResponse is the response from Single Payment
type SinglePaymentResponse struct {
	BaseResponse
	CustomerID  string            `json:"customerID"`
	ProductID   string            `json:"productID"`
	ReferenceID string            `json:"referenceID"`
	Data        SinglePaymentData `json:"data"`
}

// CheckStatusResponse is the response from Check Status
type CheckStatusResponse struct {
	BaseResponse
	CustomerID  string      `json:"customerID"`
	ProductID   string      `json:"productID"`
	ReferenceID string      `json:"referenceID"`
	Data        PaymentData `json:"data"`
}

// PriceListItem represents a product in price list
type PriceListItem struct {
	ProductID   string `json:"productID"`
	ProductName string `json:"productName"`
	Category    string `json:"category,omitempty"`
	Brand       string `json:"brand,omitempty"`
	Price       int    `json:"price,omitempty"`
	Admin       int    `json:"admin,omitempty"`
	Status      string `json:"status"`
}

// PriceListResponse is the response from Price List
type PriceListResponse struct {
	BaseResponse
	Data []PriceListItem `json:"data"`
}

// CallbackPayload represents the callback from Kiosbank
type CallbackPayload struct {
	MerchantID  string          `json:"merchantID"`
	ProductID   string          `json:"productID"`
	CustomerID  string          `json:"customerID"`
	ReferenceID string          `json:"referenceID"`
	RC          string          `json:"rc"`
	Status      string          `json:"status"`
	Message     string          `json:"message,omitempty"`
	Data        json.RawMessage `json:"data,omitempty"`
}
