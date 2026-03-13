package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_api/internal/middleware"
	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/internal/utils"
)

type TransferHandler struct {
	transferService *service.TransferService
}

func NewTransferHandler(transferService *service.TransferService) *TransferHandler {
	return &TransferHandler{transferService: transferService}
}

func (h *TransferHandler) CreateInquiry(c *gin.Context) {
	var req service.CreateTransferInquiryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "MISSING_FIELD", "Invalid request body")
		return
	}

	client := middleware.GetClient(c)
	if client == nil {
		utils.Error(c, http.StatusUnauthorized, "INVALID_TOKEN", "Unauthorized")
		return
	}

	resp, err := h.transferService.Inquiry(c.Request.Context(), &req, client, middleware.IsSandbox(c))
	if err != nil {
		h.handleError(c, err)
		return
	}

	utils.Success(c, http.StatusOK, "Inquiry successful", resp)
}

func (h *TransferHandler) CreateTransfer(c *gin.Context) {
	var req service.CreateTransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, http.StatusBadRequest, "MISSING_FIELD", "Invalid request body")
		return
	}

	client := middleware.GetClient(c)
	if client == nil {
		utils.Error(c, http.StatusUnauthorized, "INVALID_TOKEN", "Unauthorized")
		return
	}

	resp, err := h.transferService.Execute(c.Request.Context(), &req, client, middleware.IsSandbox(c))
	if err != nil {
		h.handleError(c, err)
		return
	}

	utils.Success(c, http.StatusCreated, "Transfer submitted", resp)
}

func (h *TransferHandler) GetTransfer(c *gin.Context) {
	resp, err := h.transferService.GetTransfer(c.Request.Context(), c.Param("transferId"), c.GetInt("client_id"))
	if err != nil {
		h.handleError(c, err)
		return
	}

	utils.Success(c, http.StatusOK, "Transfer retrieved", resp)
}

func (h *TransferHandler) handleError(c *gin.Context, err error) {
	var transferErr *service.TransferServiceError
	if errors.As(err, &transferErr) {
		utils.Error(c, transferErr.HTTPStatus, transferErr.Code, transferErr.Message)
		return
	}

	utils.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
}
