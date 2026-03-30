package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_api/internal/service"
)

type BRIConnectorHandler struct {
	connectorService *service.BRIConnectorService
}

func NewBRIConnectorHandler(connectorService *service.BRIConnectorService) *BRIConnectorHandler {
	return &BRIConnectorHandler{connectorService: connectorService}
}

func (h *BRIConnectorHandler) CreateAccessToken(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeBRISNAPResponse(c, http.StatusBadRequest, "4007300", "Invalid request body")
		return
	}

	resp, err := h.connectorService.IssueAccessToken(c.Request.Context(), c.Request.Header, body)
	if err != nil {
		h.handleError(c, err, "5007300", "Internal server error")
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *BRIConnectorHandler) HandleVAPaymentNotify(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeBRISNAPResponse(c, http.StatusBadRequest, "4003400", "Invalid request body")
		return
	}

	if err := h.connectorService.HandleVAPaymentNotify(c.Request.Context(), c.Request.Header, c.Request.URL.Path, body); err != nil {
		h.handleError(c, err, "5003400", "Internal server error")
		return
	}

	writeBRISNAPResponse(c, http.StatusOK, "2003400", "Successful")
}

func (h *BRIConnectorHandler) handleError(c *gin.Context, err error, fallbackCode, fallbackMessage string) {
	var connectorErr *service.BRIConnectorError
	if errors.As(err, &connectorErr) {
		writeBRISNAPResponse(c, connectorErr.HTTPStatus, connectorErr.ResponseCode, connectorErr.ResponseMessage)
		return
	}

	writeBRISNAPResponse(c, http.StatusInternalServerError, fallbackCode, fallbackMessage)
}

func writeBRISNAPResponse(c *gin.Context, httpStatus int, responseCode, responseMessage string) {
	c.JSON(httpStatus, service.BRIAckResponse{
		ResponseCode:    responseCode,
		ResponseMessage: responseMessage,
	})
}
