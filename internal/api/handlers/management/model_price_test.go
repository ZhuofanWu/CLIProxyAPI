package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestModelPriceCRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, configPath := newModelPriceTestHandler(t)

	putRec := httptest.NewRecorder()
	putCtx, _ := gin.CreateTestContext(putRec)
	putReq := httptest.NewRequest(http.MethodPut, "/v0/management/model-price", strings.NewReader(`{"model_price":[{"name":"gpt-4o","input":5,"output":15,"cache_read":2.5}]}`))
	putReq.Header.Set("Content-Type", "application/json")
	putCtx.Request = putReq
	h.PutModelPrice(putCtx)

	if putRec.Code != http.StatusOK {
		t.Fatalf("expected PUT status %d, got %d with body %s", http.StatusOK, putRec.Code, putRec.Body.String())
	}
	if len(h.cfg.ModelPrice) != 1 {
		t.Fatalf("expected 1 item after PUT, got %d", len(h.cfg.ModelPrice))
	}
	if got := h.cfg.ModelPrice[0].CacheRead.String(); got != "2.500" {
		t.Fatalf("expected cache_read 2.500 after PUT, got %s", got)
	}

	patchRec := httptest.NewRecorder()
	patchCtx, _ := gin.CreateTestContext(patchRec)
	patchReq := httptest.NewRequest(http.MethodPatch, "/v0/management/model-price", strings.NewReader(`{"name":"gpt-4o","value":{"output":16.125}}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchCtx.Request = patchReq
	h.PatchModelPrice(patchCtx)

	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected PATCH status %d, got %d with body %s", http.StatusOK, patchRec.Code, patchRec.Body.String())
	}
	if got := h.cfg.ModelPrice[0].Output.String(); got != "16.125" {
		t.Fatalf("expected output 16.125 after PATCH, got %s", got)
	}

	addRec := httptest.NewRecorder()
	addCtx, _ := gin.CreateTestContext(addRec)
	addReq := httptest.NewRequest(http.MethodPatch, "/v0/management/model-price", strings.NewReader(`{"name":"gpt-5","value":{"name":"gpt-5","input":1,"output":2,"cache_read":0.5}}`))
	addReq.Header.Set("Content-Type", "application/json")
	addCtx.Request = addReq
	h.PatchModelPrice(addCtx)

	if addRec.Code != http.StatusOK {
		t.Fatalf("expected PATCH add status %d, got %d with body %s", http.StatusOK, addRec.Code, addRec.Body.String())
	}
	if len(h.cfg.ModelPrice) != 2 {
		t.Fatalf("expected 2 items after add, got %d", len(h.cfg.ModelPrice))
	}

	getRec := httptest.NewRecorder()
	getCtx, _ := gin.CreateTestContext(getRec)
	getReq := httptest.NewRequest(http.MethodGet, "/v0/management/model-price", nil)
	getCtx.Request = getReq
	h.GetModelPrice(getCtx)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected GET status %d, got %d with body %s", http.StatusOK, getRec.Code, getRec.Body.String())
	}
	var payload struct {
		ModelPrice []config.ModelPriceItem `json:"model_price"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if len(payload.ModelPrice) != 2 {
		t.Fatalf("expected GET to return 2 items, got %d", len(payload.ModelPrice))
	}

	deleteRec := httptest.NewRecorder()
	deleteCtx, _ := gin.CreateTestContext(deleteRec)
	deleteReq := httptest.NewRequest(http.MethodDelete, "/v0/management/model-price?name=gpt-5", nil)
	deleteCtx.Request = deleteReq
	h.DeleteModelPrice(deleteCtx)

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected DELETE status %d, got %d with body %s", http.StatusOK, deleteRec.Code, deleteRec.Body.String())
	}
	if len(h.cfg.ModelPrice) != 1 {
		t.Fatalf("expected 1 item after DELETE, got %d", len(h.cfg.ModelPrice))
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "output: 16.125") {
		t.Fatalf("expected persisted config to contain updated output, got:\n%s", text)
	}
	if strings.Contains(text, "name: gpt-5") {
		t.Fatalf("expected deleted model to be absent from persisted config, got:\n%s", text)
	}
}

func TestPatchModelPriceRejectsIncompleteNewItem(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _ := newModelPriceTestHandler(t)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPatch, "/v0/management/model-price", strings.NewReader(`{"name":"gpt-4o","value":{"input":5}}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	h.PatchModelPrice(ctx)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected PATCH status %d, got %d with body %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func newModelPriceTestHandler(t *testing.T) (*Handler, string) {
	t.Helper()
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	initialConfig := strings.Join([]string{
		"host: \"\"",
		"remote-management:",
		"  secret-key: \"\"",
		"model_price: []",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(initialConfig), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return NewHandler(cfg, configPath, nil), configPath
}
