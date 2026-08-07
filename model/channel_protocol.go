package model

import (
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

func parseSupportedEndpointTypes(value *string) []constant.EndpointType {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}

	parts := strings.Split(*value, ",")
	endpointTypes := make([]constant.EndpointType, 0, len(parts))
	for _, part := range parts {
		if endpointType := constant.EndpointType(strings.TrimSpace(part)); endpointType != "" {
			endpointTypes = append(endpointTypes, endpointType)
		}
	}
	return endpointTypes
}

func (channel *Channel) GetSupportedEndpointTypes() []constant.EndpointType {
	if channel == nil {
		return nil
	}
	return parseSupportedEndpointTypes(channel.SupportedEndpoints)
}

func (channel *Channel) ValidateSupportedEndpoints() error {
	seen := make(map[constant.EndpointType]struct{})
	for _, endpointType := range channel.GetSupportedEndpointTypes() {
		if !constant.IsValidEndpointType(endpointType) {
			return fmt.Errorf("unsupported endpoint protocol: %s", endpointType)
		}
		if _, exists := seen[endpointType]; exists {
			return fmt.Errorf("duplicate endpoint protocol: %s", endpointType)
		}
		seen[endpointType] = struct{}{}
	}
	return nil
}

func supportsRequestPath(channelType int, supportedEndpoints []constant.EndpointType, advancedCustom *dto.AdvancedCustomConfig, requestPath string, model string) bool {
	if requestPath == "" {
		return true
	}
	if len(supportedEndpoints) > 0 {
		endpointType, ok := common.GetEndpointTypeByPath(requestPath)
		if !ok || !slices.Contains(supportedEndpoints, endpointType) {
			return false
		}
	}
	if channelType != constant.ChannelTypeAdvancedCustom {
		return true
	}
	return advancedCustom != nil && advancedCustom.SupportsPathForModel(requestPath, model)
}

func (channel *Channel) SupportsRequestPath(requestPath string, model string) bool {
	if channel == nil {
		return false
	}
	var advancedCustom *dto.AdvancedCustomConfig
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		advancedCustom = channel.GetOtherSettings().AdvancedCustom
	}
	return supportsRequestPath(
		channel.Type,
		channel.GetSupportedEndpointTypes(),
		advancedCustom,
		requestPath,
		model,
	)
}
