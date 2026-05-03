package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/GTDGit/gtd_api/internal/models"
)

func newTestRouter(scope string, client *models.Client) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x",
		func(c *gin.Context) {
			if client != nil {
				c.Set("client", client)
			}
			c.Next()
		},
		RequireScope(scope),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)
	return r
}

func TestRequireScope_AllowsWhenScopePresent(t *testing.T) {
	client := &models.Client{Scopes: []string{models.ScopePPOB, models.ScopePayment}}
	r := newTestRouter(models.ScopePayment, client)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestRequireScope_RejectsWhenScopeMissing(t *testing.T) {
	client := &models.Client{Scopes: []string{models.ScopePPOB}}
	r := newTestRouter(models.ScopePayment, client)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errInfo, _ := body["error"].(map[string]any)
	if code, _ := errInfo["code"].(string); code != "INSUFFICIENT_SCOPE" {
		t.Fatalf("expected INSUFFICIENT_SCOPE, got %v", errInfo)
	}
	if msg, _ := errInfo["message"].(string); !strings.Contains(msg, "payment") {
		t.Fatalf("expected message to mention payment, got %q", msg)
	}
}

func TestRequireScope_RejectsWhenNoClientInContext(t *testing.T) {
	r := newTestRouter(models.ScopePPOB, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
