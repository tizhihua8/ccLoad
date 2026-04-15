package app

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// ==================== 冷却管理 ====================
// 从admin.go拆分冷却管理,遵循SRP原则

// HandleSetChannelCooldown 设置渠道级别冷却
func (s *Server) HandleSetChannelCooldown(c *gin.Context) {
	id, err := ParseInt64Param(c, "id")
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid channel ID")
		return
	}

	var req CooldownRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, err)
		return
	}

	if req.DurationMs == 0 {
		// 清除冷却：数据库重置 + 内存缓存清除
		if err := s.store.ResetChannelCooldown(c.Request.Context(), id); err != nil {
			RespondError(c, http.StatusInternalServerError, err)
			return
		}
		if s.cooldownManager != nil {
			_ = s.cooldownManager.ClearChannelCooldown(c.Request.Context(), id)
		}
		s.invalidateCooldownCache()
		s.InvalidateChannelListCache()
		RespondJSON(c, http.StatusOK, gin.H{"message": "渠道冷却已清除"})
	} else {
		until := time.Now().Add(time.Duration(req.DurationMs) * time.Millisecond)
		if err := s.store.SetChannelCooldown(c.Request.Context(), id, until); err != nil {
			RespondError(c, http.StatusInternalServerError, err)
			return
		}
		s.invalidateCooldownCache()
		s.InvalidateChannelListCache()
		RespondJSON(c, http.StatusOK, gin.H{"message": fmt.Sprintf("渠道已冷却 %d 毫秒", req.DurationMs)})
	}
}

// HandleSetKeyCooldown 设置Key级别冷却
func (s *Server) HandleSetKeyCooldown(c *gin.Context) {
	id, err := ParseInt64Param(c, "id")
	if err != nil {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid channel ID")
		return
	}

	keyIndexStr := c.Param("keyIndex")
	keyIndex, err := strconv.Atoi(keyIndexStr)
	if err != nil || keyIndex < 0 {
		RespondErrorMsg(c, http.StatusBadRequest, "invalid key index")
		return
	}

	var req CooldownRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, err)
		return
	}

	if req.DurationMs == 0 {
		if err := s.store.ResetKeyCooldown(c.Request.Context(), id, keyIndex); err != nil {
			RespondError(c, http.StatusInternalServerError, err)
			return
		}
		s.invalidateCooldownCache()
		s.InvalidateAPIKeysCache(id)
		RespondJSON(c, http.StatusOK, gin.H{"message": fmt.Sprintf("Key #%d 冷却已清除", keyIndex+1)})
	} else {
		until := time.Now().Add(time.Duration(req.DurationMs) * time.Millisecond)
		if err := s.store.SetKeyCooldown(c.Request.Context(), id, keyIndex, until); err != nil {
			RespondError(c, http.StatusInternalServerError, err)
			return
		}
		s.invalidateCooldownCache()
		s.InvalidateAPIKeysCache(id)
		RespondJSON(c, http.StatusOK, gin.H{"message": fmt.Sprintf("Key #%d 已冷却 %d 毫秒", keyIndex+1, req.DurationMs)})
	}
}
