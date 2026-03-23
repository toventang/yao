package llm

import (
	"github.com/yaoapp/gou/connector"
	goullm "github.com/yaoapp/gou/llm"
)

// GetCapabilities get the capabilities of a connector by connector ID
// Reads capabilities from connector's Setting()["capabilities"], with fallback to defaults.
func GetCapabilities(connectorID string) *goullm.Capabilities {
	if connectorID == "" {
		return getDefaultCapabilities()
	}

	conn, err := connector.Select(connectorID)
	if err != nil {
		return getDefaultCapabilities()
	}

	return GetCapabilitiesFromConn(conn)
}

// GetCapabilitiesFromConn get the capabilities from a connector instance
func GetCapabilitiesFromConn(conn connector.Connector) *goullm.Capabilities {
	if conn == nil {
		return getDefaultCapabilities()
	}

	settings := conn.Setting()
	if settings != nil {
		if caps, ok := settings["capabilities"]; ok {
			if capabilities, ok := caps.(*goullm.Capabilities); ok {
				return capabilities
			}
			if capabilities, ok := caps.(goullm.Capabilities); ok {
				return &capabilities
			}
		}
	}

	return getDefaultCapabilities()
}

// getDefaultCapabilities returns minimal default capabilities
func getDefaultCapabilities() *goullm.Capabilities {
	return &goullm.Capabilities{
		Vision:                false,
		ToolCalls:             false,
		Audio:                 false,
		Reasoning:             false,
		Streaming:             false,
		JSON:                  false,
		Multimodal:            false,
		TemperatureAdjustable: true,
	}
}

// GetCapabilitiesMap get capabilities as map[string]interface{} for API responses
func GetCapabilitiesMap(connectorID string) map[string]interface{} {
	caps := GetCapabilities(connectorID)
	if caps == nil {
		return nil
	}

	return ToMap(caps)
}

// ToMap converts Capabilities to map[string]interface{}
func ToMap(caps *goullm.Capabilities) map[string]interface{} {
	if caps == nil {
		return nil
	}

	result := make(map[string]interface{})

	if caps.Vision != nil {
		result["vision"] = caps.Vision
	}

	result["audio"] = caps.Audio
	result["stt"] = caps.STT
	result["tool_calls"] = caps.ToolCalls
	result["reasoning"] = caps.Reasoning
	result["streaming"] = caps.Streaming
	result["json"] = caps.JSON
	result["multimodal"] = caps.Multimodal
	result["temperature_adjustable"] = caps.TemperatureAdjustable

	return result
}
