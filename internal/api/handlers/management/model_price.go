package management

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func (h *Handler) GetModelPrice(c *gin.Context) {
	c.JSON(200, gin.H{"model_price": h.cfg.ModelPrice})
}

func (h *Handler) PutModelPrice(c *gin.Context) {
	data, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to read body"})
		return
	}
	var items []config.ModelPriceItem
	if err = json.Unmarshal(data, &items); err != nil {
		var body struct {
			ModelPrice *[]config.ModelPriceItem `json:"model_price"`
		}
		if errBody := json.Unmarshal(data, &body); errBody != nil {
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}
		if body.ModelPrice == nil {
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}
		items = *body.ModelPrice
	}
	h.cfg.ModelPrice = append([]config.ModelPriceItem(nil), items...)
	h.cfg.SanitizeModelPrice()
	h.persist(c)
}

func (h *Handler) PatchModelPrice(c *gin.Context) {
	type modelPricePatch struct {
		Name      *string                 `json:"name"`
		Input     *config.ModelPriceValue `json:"input"`
		Output    *config.ModelPriceValue `json:"output"`
		CacheRead *config.ModelPriceValue `json:"cache_read"`
	}
	var body struct {
		Index *int             `json:"index"`
		Name  *string          `json:"name"`
		Value *modelPricePatch `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Value == nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}

	targetIndex := -1
	if body.Index != nil && *body.Index >= 0 && *body.Index < len(h.cfg.ModelPrice) {
		targetIndex = *body.Index
	}
	if targetIndex == -1 && body.Name != nil {
		targetName := strings.TrimSpace(*body.Name)
		for i := range h.cfg.ModelPrice {
			if h.cfg.ModelPrice[i].Name == targetName {
				targetIndex = i
				break
			}
		}
	}

	if targetIndex == -1 {
		newName := ""
		if body.Value.Name != nil {
			newName = strings.TrimSpace(*body.Value.Name)
		}
		if newName == "" && body.Name != nil {
			newName = strings.TrimSpace(*body.Name)
		}
		if newName == "" || body.Value.Input == nil || body.Value.Output == nil || body.Value.CacheRead == nil {
			c.JSON(400, gin.H{"error": "missing fields for new model_price item"})
			return
		}
		h.cfg.ModelPrice = append(h.cfg.ModelPrice, config.ModelPriceItem{
			Name:      newName,
			Input:     *body.Value.Input,
			Output:    *body.Value.Output,
			CacheRead: *body.Value.CacheRead,
		})
		h.cfg.SanitizeModelPrice()
		h.persist(c)
		return
	}

	entry := h.cfg.ModelPrice[targetIndex]
	if body.Value.Name != nil {
		trimmed := strings.TrimSpace(*body.Value.Name)
		if trimmed == "" {
			c.JSON(400, gin.H{"error": "name must not be empty"})
			return
		}
		entry.Name = trimmed
	}
	if body.Value.Input != nil {
		entry.Input = *body.Value.Input
	}
	if body.Value.Output != nil {
		entry.Output = *body.Value.Output
	}
	if body.Value.CacheRead != nil {
		entry.CacheRead = *body.Value.CacheRead
	}
	h.cfg.ModelPrice[targetIndex] = entry
	h.cfg.SanitizeModelPrice()
	h.persist(c)
}

func (h *Handler) DeleteModelPrice(c *gin.Context) {
	if name := strings.TrimSpace(c.Query("name")); name != "" {
		out := make([]config.ModelPriceItem, 0, len(h.cfg.ModelPrice))
		for _, item := range h.cfg.ModelPrice {
			if item.Name != name {
				out = append(out, item)
			}
		}
		if len(out) == len(h.cfg.ModelPrice) {
			c.JSON(404, gin.H{"error": "item not found"})
			return
		}
		h.cfg.ModelPrice = out
		h.persist(c)
		return
	}
	if idxStr := c.Query("index"); idxStr != "" {
		var idx int
		if _, err := fmt.Sscanf(idxStr, "%d", &idx); err == nil && idx >= 0 && idx < len(h.cfg.ModelPrice) {
			h.cfg.ModelPrice = append(h.cfg.ModelPrice[:idx], h.cfg.ModelPrice[idx+1:]...)
			h.persist(c)
			return
		}
	}
	c.JSON(400, gin.H{"error": "missing name or index"})
}
