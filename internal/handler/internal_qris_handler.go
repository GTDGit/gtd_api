package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_api/internal/utils"
	"github.com/GTDGit/gtd_api/pkg/pakailink"
)

// InternalQRISHandler exposes service-to-service endpoints for the gateway to
// drive Pakailink QRIS registration + static QR generation. The Pakailink client
// only lives in the api service; the gateway proxies to these routes (guarded by
// the internal-token middleware) instead of holding provider credentials itself.
type InternalQRISHandler struct {
	pakailink   *pakailink.Client
	callbackURL string
}

func NewInternalQRISHandler(client *pakailink.Client, callbackURL string) *InternalQRISHandler {
	return &InternalQRISHandler{pakailink: client, callbackURL: callbackURL}
}

type pakailinkRegisterRequest struct {
	PartnerReferenceNo string `json:"partnerReferenceNo"`
	MerchantName       string `json:"merchantName"`
	MerchantEmail      string `json:"merchantEmail"`
	StoreApplication   string `json:"storeApplicationName"`
	StoreWebsite       string `json:"storeWebsite"`
	StoreType          string `json:"storeType"`
	StoreName          string `json:"storeName"`
	Omzet              string `json:"omzet"`
	Address            string `json:"address"`
	City               string `json:"city"`
	PostalCode         string `json:"postalCode"`
	Province           string `json:"province"`
	Country            string `json:"country"`
	OwnerFirstName     string `json:"ownerFirstName"`
	OwnerLastName      string `json:"ownerLastName"`
	OwnerEmail         string `json:"ownerEmail"`
	OwnerPhone         string `json:"ownerPhone"`
	OwnerIDNumber      string `json:"ownerIdNumber"`
	OwnerTaxID         string `json:"ownerTaxId"`
	OwnerDateOfBirth   string `json:"ownerDateOfBirth"`
	OwnerPlaceOfBirth  string `json:"ownerPlaceOfBirth"`
}

// RegisterPakailink registers a static QRIS merchant via Pakailink Service 49.
func (h *InternalQRISHandler) RegisterPakailink(c *gin.Context) {
	if h.pakailink == nil {
		utils.Error(c, http.StatusServiceUnavailable, "PROVIDER_UNAVAILABLE", "pakailink client not configured")
		return
	}
	var req pakailinkRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	partnerRef := strings.TrimSpace(req.PartnerReferenceNo)
	if partnerRef == "" {
		partnerRef = "QRISREG" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	resp, err := h.pakailink.RegisterQRISMerchant(c.Request.Context(), pakailink.RegisterQRISRequest{
		PartnerReferenceNo: partnerRef,
		MerchantName:       req.MerchantName,
		MerchantEmail:      req.MerchantEmail,
		MerchantType:       "STATIS",
		StoreApplication:   req.StoreApplication,
		StoreWebsite:       req.StoreWebsite,
		StoreType:          req.StoreType,
		StoreName:          req.StoreName,
		Omzet:              req.Omzet,
		Address:            req.Address,
		City:               req.City,
		PostalCode:         req.PostalCode,
		Province:           req.Province,
		Country:            req.Country,
		OwnerFirstName:     req.OwnerFirstName,
		OwnerLastName:      req.OwnerLastName,
		OwnerEmail:         req.OwnerEmail,
		OwnerPhone:         req.OwnerPhone,
		OwnerIDNumber:      req.OwnerIDNumber,
		OwnerTaxID:         req.OwnerTaxID,
		OwnerDateOfBirth:   req.OwnerDateOfBirth,
		OwnerPlaceOfBirth:  req.OwnerPlaceOfBirth,
	})
	if err != nil {
		utils.Error(c, http.StatusBadGateway, "PROVIDER_ERROR", err.Error())
		return
	}

	utils.Success(c, http.StatusOK, "Merchant registered", gin.H{
		"partnerReferenceNo": partnerRef,
		"detailData":         resp.DetailData,
		"raw":                resp.RawResponse,
	})
}

type pakailinkGenerateRequest struct {
	PartnerReferenceNo string `json:"partnerReferenceNo"`
	MerchantID         string `json:"merchantId"`
	StoreID            string `json:"storeId"`
	TerminalID         string `json:"terminalId"`
	MerchantName       string `json:"merchantName"`
}

// GeneratePakailink produces a STATIC QRIS string (additionalInfo.type=statis)
// and parses the returned QR so the gateway can persist the decoded fields.
func (h *InternalQRISHandler) GeneratePakailink(c *gin.Context) {
	if h.pakailink == nil {
		utils.Error(c, http.StatusServiceUnavailable, "PROVIDER_UNAVAILABLE", "pakailink client not configured")
		return
	}
	var req pakailinkGenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	partnerRef := strings.TrimSpace(req.PartnerReferenceNo)
	if partnerRef == "" {
		partnerRef = "QRISGEN" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	resp, err := h.pakailink.GenerateQRMPM(c.Request.Context(), pakailink.GenerateQRRequest{
		PartnerReferenceNo: partnerRef,
		Amount:             0, // static / open amount
		TerminalID:         req.TerminalID,
		StoreID:            req.StoreID,
		MerchantID:         req.MerchantID,
		MerchantName:       req.MerchantName,
		CallbackURL:        h.callbackURL,
		Type:               "statis",
	})
	if err != nil {
		utils.Error(c, http.StatusBadGateway, "PROVIDER_ERROR", err.Error())
		return
	}

	qrContent := strings.TrimSpace(resp.QRContent)
	out := gin.H{
		"partnerReferenceNo": partnerRef,
		"referenceNo":        resp.ReferenceNo,
		"qrContent":          qrContent,
		"terminalId":         resp.TerminalID,
	}
	if qrContent != "" {
		if info, perr := utils.ParseQRIS(qrContent); perr == nil {
			out["parsed"] = info
		}
	}
	utils.Success(c, http.StatusOK, "QRIS generated", out)
}
