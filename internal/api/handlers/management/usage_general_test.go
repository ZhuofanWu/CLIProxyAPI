package management

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

func TestGetUsageGeneral_MemoryReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{usageStats: usage.NewRequestStatistics()}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/general?range=24h", nil)

	h.GetUsageGeneral(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
