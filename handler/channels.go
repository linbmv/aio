package handler

import (
	"net/http"
	"strconv"

	"github.com/atopos31/llmio/common"
	"github.com/atopos31/llmio/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ChannelRequest Channel创建/更新请求
type ChannelRequest struct {
	Name                string                 `json:"name" binding:"required"`
	Description         string                 `json:"description"`
	ProviderID          uint                   `json:"provider_id" binding:"required"`
	SupportedProtocols  []string               `json:"supported_protocols"`
	LoadBalanceStrategy string                 `json:"load_balance_strategy"`
	Weight              int                    `json:"weight"`
	Status              string                 `json:"status"`
	ParameterOverrides  map[string]interface{} `json:"parameter_overrides"`
}

// ModelMappingRequest 模型映射创建/更新请求
type ModelMappingRequest struct {
	ChannelID          uint                   `json:"channel_id" binding:"required"`
	VirtualModel       string                 `json:"virtual_model" binding:"required"`
	ActualModel        string                 `json:"actual_model" binding:"required"`
	Protocol           string                 `json:"protocol" binding:"required"`
	ParameterOverrides map[string]interface{} `json:"parameter_overrides"`
	Weight             int                    `json:"weight"`
	Status             string                 `json:"status"`
}

// ChannelResponse Channel响应
type ChannelResponse struct {
	models.Channel
	ProviderName string  `json:"provider_name"`
	ErrorRate    float64 `json:"error_rate"`
	SuccessRate  float64 `json:"success_rate"`
}

// GetChannels 获取Channel列表
func GetChannels(c *gin.Context) {
	var channels []models.Channel
	query := models.DB.Preload("Provider").Preload("ModelMappings")

	// 支持按Provider ID过滤
	if providerID := c.Query("provider_id"); providerID != "" {
		if id, err := strconv.ParseUint(providerID, 10, 32); err == nil {
			query = query.Where("provider_id = ?", id)
		}
	}

	// 支持按状态过滤
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Find(&channels).Error; err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "获取Channel列表失败", err.Error())
		return
	}

	// 转换为响应格式
	var responses []ChannelResponse
	for _, channel := range channels {
		response := ChannelResponse{
			Channel:      channel,
			ProviderName: channel.Provider.Name,
			ErrorRate:    channel.GetErrorRate(),
			SuccessRate:  channel.GetSuccessRate(),
		}
		responses = append(responses, response)
	}

	common.SuccessResponse(c, responses)
}

// GetChannel 获取单个Channel
func GetChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "无效的Channel ID", err.Error())
		return
	}

	var channel models.Channel
	if err := models.DB.Preload("Provider").Preload("ModelMappings").First(&channel, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			common.ErrorResponse(c, http.StatusNotFound, "Channel不存在", "")
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "获取Channel失败", err.Error())
		return
	}

	response := ChannelResponse{
		Channel:      channel,
		ProviderName: channel.Provider.Name,
		ErrorRate:    channel.GetErrorRate(),
		SuccessRate:  channel.GetSuccessRate(),
	}

	common.SuccessResponse(c, response)
}

// CreateChannel 创建Channel
func CreateChannel(c *gin.Context) {
	var req ChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "请求参数错误", err.Error())
		return
	}

	// 验证Provider是否存在
	var provider models.Provider
	if err := models.DB.First(&provider, req.ProviderID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			common.ErrorResponse(c, http.StatusBadRequest, "Provider不存在", "")
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "验证Provider失败", err.Error())
		return
	}

	// 检查Channel名称是否重复
	var existingChannel models.Channel
	if err := models.DB.Where("name = ?", req.Name).First(&existingChannel).Error; err == nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Channel名称已存在", "")
		return
	}

	// 设置默认值
	if req.LoadBalanceStrategy == "" {
		req.LoadBalanceStrategy = "error_aware"
	}
	if req.Weight == 0 {
		req.Weight = 1
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if req.SupportedProtocols == nil {
		req.SupportedProtocols = []string{"openai", "anthropic"}
	}
	if req.ParameterOverrides == nil {
		req.ParameterOverrides = make(map[string]interface{})
	}

	channel := models.Channel{
		Name:                req.Name,
		Description:         req.Description,
		ProviderID:          req.ProviderID,
		SupportedProtocols:  req.SupportedProtocols,
		LoadBalanceStrategy: req.LoadBalanceStrategy,
		Weight:              req.Weight,
		Status:              req.Status,
		ParameterOverrides:  req.ParameterOverrides,
	}

	if err := models.DB.Create(&channel).Error; err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "创建Channel失败", err.Error())
		return
	}

	// 重新加载关联数据
	models.DB.Preload("Provider").First(&channel, channel.ID)

	response := ChannelResponse{
		Channel:      channel,
		ProviderName: channel.Provider.Name,
		ErrorRate:    0,
		SuccessRate:  0,
	}

	common.SuccessResponse(c, response)
}

// UpdateChannel 更新Channel
func UpdateChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "无效的Channel ID", err.Error())
		return
	}

	var req ChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "请求参数错误", err.Error())
		return
	}

	var channel models.Channel
	if err := models.DB.First(&channel, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			common.ErrorResponse(c, http.StatusNotFound, "Channel不存在", "")
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "获取Channel失败", err.Error())
		return
	}

	// 验证Provider是否存在
	var provider models.Provider
	if err := models.DB.First(&provider, req.ProviderID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			common.ErrorResponse(c, http.StatusBadRequest, "Provider不存在", "")
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "验证Provider失败", err.Error())
		return
	}

	// 检查名称是否与其他Channel重复
	if req.Name != channel.Name {
		var existingChannel models.Channel
		if err := models.DB.Where("name = ? AND id != ?", req.Name, id).First(&existingChannel).Error; err == nil {
			common.ErrorResponse(c, http.StatusBadRequest, "Channel名称已存在", "")
			return
		}
	}

	// 更新字段
	channel.Name = req.Name
	channel.Description = req.Description
	channel.ProviderID = req.ProviderID
	channel.SupportedProtocols = req.SupportedProtocols
	channel.LoadBalanceStrategy = req.LoadBalanceStrategy
	channel.Weight = req.Weight
	channel.Status = req.Status
	channel.ParameterOverrides = req.ParameterOverrides

	if err := models.DB.Save(&channel).Error; err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "更新Channel失败", err.Error())
		return
	}

	// 重新加载关联数据
	models.DB.Preload("Provider").First(&channel, channel.ID)

	response := ChannelResponse{
		Channel:      channel,
		ProviderName: channel.Provider.Name,
		ErrorRate:    channel.GetErrorRate(),
		SuccessRate:  channel.GetSuccessRate(),
	}

	common.SuccessResponse(c, response)
}

// DeleteChannel 删除Channel
func DeleteChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "无效的Channel ID", err.Error())
		return
	}

	var channel models.Channel
	if err := models.DB.First(&channel, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			common.ErrorResponse(c, http.StatusNotFound, "Channel不存在", "")
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "获取Channel失败", err.Error())
		return
	}

	// 删除关联的模型映射
	if err := models.DB.Where("channel_id = ?", id).Delete(&models.ModelMapping{}).Error; err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "删除关联的模型映射失败", err.Error())
		return
	}

	// 删除Channel
	if err := models.DB.Delete(&channel).Error; err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "删除Channel失败", err.Error())
		return
	}

	common.SuccessResponse(c, gin.H{"message": "Channel删除成功"})
}

// GetChannelStats 获取Channel统计信息
func GetChannelStats(c *gin.Context) {
	var stats []models.ChannelStats

	query := `
		SELECT
			id as channel_id,
			name as channel_name,
			total_requests,
			success_requests,
			error_requests,
			CASE
				WHEN total_requests = 0 THEN 0
				ELSE CAST(error_requests AS FLOAT) / total_requests
			END as error_rate,
			avg_response_time,
			status,
			last_used_at
		FROM channels
		ORDER BY total_requests DESC
	`

	if err := models.DB.Raw(query).Scan(&stats).Error; err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "获取Channel统计失败", err.Error())
		return
	}

	common.SuccessResponse(c, stats)
}

// ResetChannelCooldown 重置Channel冷却状态
func ResetChannelCooldown(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "无效的Channel ID", err.Error())
		return
	}

	var channel models.Channel
	if err := models.DB.First(&channel, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			common.ErrorResponse(c, http.StatusNotFound, "Channel不存在", "")
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "获取Channel失败", err.Error())
		return
	}

	// 重置冷却状态
	channel.ClearCooldown()

	if err := models.DB.Save(&channel).Error; err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "重置冷却状态失败", err.Error())
		return
	}

	common.SuccessResponse(c, gin.H{"message": "冷却状态重置成功"})
}

// GetModelMappings 获取模型映射列表
func GetModelMappings(c *gin.Context) {
	var mappings []models.ModelMapping
	query := models.DB.Preload("Channel").Preload("Channel.Provider")

	// 支持按Channel ID过滤
	if channelID := c.Query("channel_id"); channelID != "" {
		if id, err := strconv.ParseUint(channelID, 10, 32); err == nil {
			query = query.Where("channel_id = ?", id)
		}
	}

	// 支持按虚拟模型过滤
	if virtualModel := c.Query("virtual_model"); virtualModel != "" {
		query = query.Where("virtual_model LIKE ?", "%"+virtualModel+"%")
	}

	if err := query.Find(&mappings).Error; err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "获取模型映射列表失败", err.Error())
		return
	}

	common.SuccessResponse(c, mappings)
}

// CreateModelMapping 创建模型映射
func CreateModelMapping(c *gin.Context) {
	var req ModelMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "请求参数错误", err.Error())
		return
	}

	// 验证Channel是否存在
	var channel models.Channel
	if err := models.DB.First(&channel, req.ChannelID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			common.ErrorResponse(c, http.StatusBadRequest, "Channel不存在", "")
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "验证Channel失败", err.Error())
		return
	}

	// 检查虚拟模型是否已存在
	var existingMapping models.ModelMapping
	if err := models.DB.Where("virtual_model = ? AND channel_id = ?", req.VirtualModel, req.ChannelID).First(&existingMapping).Error; err == nil {
		common.ErrorResponse(c, http.StatusBadRequest, "该Channel中虚拟模型已存在", "")
		return
	}

	// 设置默认值
	if req.Weight == 0 {
		req.Weight = 1
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if req.ParameterOverrides == nil {
		req.ParameterOverrides = make(map[string]interface{})
	}

	mapping := models.ModelMapping{
		ChannelID:          req.ChannelID,
		VirtualModel:       req.VirtualModel,
		ActualModel:        req.ActualModel,
		Protocol:           req.Protocol,
		ParameterOverrides: req.ParameterOverrides,
		Weight:             req.Weight,
		Status:             req.Status,
	}

	if err := models.DB.Create(&mapping).Error; err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "创建模型映射失败", err.Error())
		return
	}

	// 重新加载关联数据
	models.DB.Preload("Channel").Preload("Channel.Provider").First(&mapping, mapping.ID)

	common.SuccessResponse(c, mapping)
}

// UpdateModelMapping 更新模型映射
func UpdateModelMapping(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "无效的模型映射ID", err.Error())
		return
	}

	var req ModelMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "请求参数错误", err.Error())
		return
	}

	var mapping models.ModelMapping
	if err := models.DB.First(&mapping, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			common.ErrorResponse(c, http.StatusNotFound, "模型映射不存在", "")
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "获取模型映射失败", err.Error())
		return
	}

	// 验证Channel是否存在
	var channel models.Channel
	if err := models.DB.First(&channel, req.ChannelID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			common.ErrorResponse(c, http.StatusBadRequest, "Channel不存在", "")
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "验证Channel失败", err.Error())
		return
	}

	// 检查虚拟模型是否与其他映射重复
	if req.VirtualModel != mapping.VirtualModel || req.ChannelID != mapping.ChannelID {
		var existingMapping models.ModelMapping
		if err := models.DB.Where("virtual_model = ? AND channel_id = ? AND id != ?", req.VirtualModel, req.ChannelID, id).First(&existingMapping).Error; err == nil {
			common.ErrorResponse(c, http.StatusBadRequest, "该Channel中虚拟模型已存在", "")
			return
		}
	}

	// 更新字段
	mapping.ChannelID = req.ChannelID
	mapping.VirtualModel = req.VirtualModel
	mapping.ActualModel = req.ActualModel
	mapping.Protocol = req.Protocol
	mapping.ParameterOverrides = req.ParameterOverrides
	mapping.Weight = req.Weight
	mapping.Status = req.Status

	if err := models.DB.Save(&mapping).Error; err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "更新模型映射失败", err.Error())
		return
	}

	// 重新加载关联数据
	models.DB.Preload("Channel").Preload("Channel.Provider").First(&mapping, mapping.ID)

	common.SuccessResponse(c, mapping)
}

// DeleteModelMapping 删除模型映射
func DeleteModelMapping(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "无效的模型映射ID", err.Error())
		return
	}

	var mapping models.ModelMapping
	if err := models.DB.First(&mapping, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			common.ErrorResponse(c, http.StatusNotFound, "模型映射不存在", "")
			return
		}
		common.ErrorResponse(c, http.StatusInternalServerError, "获取模型映射失败", err.Error())
		return
	}

	if err := models.DB.Delete(&mapping).Error; err != nil {
		common.ErrorResponse(c, http.StatusInternalServerError, "删除模型映射失败", err.Error())
		return
	}

	common.SuccessResponse(c, gin.H{"message": "模型映射删除成功"})
}
