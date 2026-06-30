package handlers

import (
	"fmt"
	"strings"
)

// FaultCondition represents a detected fault condition
type FaultCondition struct {
	Code        string
	Description string
	Severity    string // "critical", "warning", "info"
}

// faultDescriptions maps each fault code to a human-readable description.
// Descriptions must NOT contain commas (codes are comma-separated upstream).
var faultDescriptions = map[string]string{
	// Generic / shared
	"PRESSURE_LOW":  "Pressure below safe minimum",
	"PRESSURE_HIGH": "Pressure above safe maximum",
	"TEMP_HIGH":     "Temperature above safe maximum",
	"TEMP_LOW":      "Temperature below safe minimum",
	// AP model
	"LP_LOW":         "Low-side pressure too low",
	"LP_HIGH":        "Low-side pressure too high",
	"HP_LOW":         "High-side pressure too low",
	"HP_HIGH":        "High-side pressure too high",
	"T1_HIGH":        "Cold air (T1) too high",
	"T1_LOW":         "Cold air (T1) too low",
	"T2_HIGH":        "Ambient (T2) too high",
	"T2_LOW":         "Ambient (T2) too low",
	"BLOWER_STALLED": "Blower stalled in manual mode",
	// T model
	"LP_LOW_T":      "Low-side pressure too low",
	"LP_HIGH_T":     "Low-side pressure too high",
	"HP_LOW_T":      "High-side pressure too low",
	"HP_HIGH_T":     "High-side pressure too high",
	"T0_HIGH":       "Air outlet (T0) too high",
	"T0_LOW":        "Air outlet (T0) too low",
	"HOT_VALVE_MIN": "Hot gas valve running below minimum",
	"AHT_VALVE_MIN": "After-heat valve running below minimum",
	// E model
	"LP_LOW_E":   "Low-side pressure too low",
	"LP_HIGH_E":  "Low-side pressure too high",
	"HP_LOW_E":   "High-side pressure too low",
	"HP_HIGH_E":  "High-side pressure too high",
	"HEATER_LOW": "Heater running below minimum",
	// EP model (German)
	"LP_EP_LOW":   "Low-side pressure too low",
	"LP_EP_HIGH":  "Low-side pressure too high",
	"HP_EP_LOW":   "High-side pressure too low",
	"HP_EP_HIGH":  "High-side pressure too high",
	"OUTLET_HIGH": "Air outlet temperature too high",
	"OUTLET_LOW":  "Air outlet temperature too low",
	// GTPL-124
	"LP_LOW_124":     "Low-side pressure too low",
	"LP_HIGH_124":    "Low-side pressure too high",
	"TEMP_ABOVE_SET": "Outlet temperature above set point",
	"TEMP_BELOW_SET": "Outlet temperature below set point",
	// Thailand T
	"COND_FAN_LOW": "Condenser fan running below minimum",
	// GTPL-118
	"COND_FAN_118_LOW":  "Condenser fan running below minimum",
	"AHT_VALVE_118_LOW": "After-heat valve running below minimum",
	"T1_ABOVE_SET_118":  "Cold air (T1) above set point",
	"T1_BELOW_SET_118":  "Cold air (T1) below set point",
}

// formatFaultsWithCode turns an ordered list of fault codes into a single
// string showing each code with its description, e.g.
// "LP_LOW: Low-side pressure too low; HP_HIGH: High-side pressure too high".
// Unknown codes (e.g. raw codes from the DB FAULT_CODE column) pass through as-is.
func formatFaultsWithCode(codes []string) string {
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if d, ok := faultDescriptions[c]; ok {
			out = append(out, c+": "+d)
		} else {
			out = append(out, c)
		}
	}
	return strings.Join(out, "; ")
}

// detectFaultConditions analyzes data values and returns fault codes if any conditions are met
func detectFaultConditions(table string, values map[string]interface{}) string {
	// Check for specific machine types
	if isAPModel(table) {
		return detectAPModelFaults(values)
	} else if isTModel(table) {
		return detectTModelFaults(values)
	} else if isEModel(table) {
		return detectEModelFaults(values)
	} else if isEPModel(table) {
		return detectEPModelFaults(values)
	} else if isGTPL124(table) {
		return detectGTPL124Faults(values)
	} else if isThailandT(table) {
		return detectThailandTFaults(values)
	} else if isGTPL118(table) {
		return detectGTPL118Faults(values)
	}
	
	// Generic fault detection
	return detectGenericFaults(values)
}

// detectAPModelFaults detects faults for AP model machines
func detectAPModelFaults(values map[string]interface{}) string {
	faults := []string{}
	
	// Check LP pressure (too low or too high)
	if lp, ok := getFloatValue(values, "LP_value"); ok {
		if lp < 1.0 {
			faults = append(faults, "LP_LOW")
		} else if lp > 5.0 {
			faults = append(faults, "LP_HIGH")
		}
	}
	
	// Check HP pressure
	if hp, ok := getFloatValue(values, "HP_value"); ok {
		if hp < 10.0 {
			faults = append(faults, "HP_LOW")
		} else if hp > 25.0 {
			faults = append(faults, "HP_HIGH")
		}
	}
	
	// Check temperatures
	if t1, ok := getFloatValue(values, "T1_temp_mean"); ok {
		if t1 > 40.0 {
			faults = append(faults, "T1_HIGH")
		} else if t1 < -10.0 {
			faults = append(faults, "T1_LOW")
		}
	}
	
	if t2, ok := getFloatValue(values, "T2_temp_mean"); ok {
		if t2 > 45.0 {
			faults = append(faults, "T2_HIGH")
		} else if t2 < -5.0 {
			faults = append(faults, "T2_LOW")
		}
	}
	
	// Check if machine is in manual mode but no operation
	if manualMode, ok := getIntValue(values, "Manual_mode"); ok && manualMode == 1 {
		// Check if any valve or blower is not operating
		if blower, ok := getFloatValue(values, "Blower_speed"); ok && blower < 10.0 {
			faults = append(faults, "BLOWER_STALLED")
		}
	}
	
	if len(faults) > 0 {
		return strings.Join(faults, ",")
	}
	
	return ""
}

// detectTModelFaults detects faults for T model machines
func detectTModelFaults(values map[string]interface{}) string {
	faults := []string{}
	
	// Check pressures
	if lp, ok := getFloatValue(values, "LP_value"); ok {
		if lp < 1.5 {
			faults = append(faults, "LP_LOW_T")
		} else if lp > 4.5 {
			faults = append(faults, "LP_HIGH_T")
		}
	}
	
	if hp, ok := getFloatValue(values, "HP_value"); ok {
		if hp < 12.0 {
			faults = append(faults, "HP_LOW_T")
		} else if hp > 22.0 {
			faults = append(faults, "HP_HIGH_T")
		}
	}
	
	// Check temperatures
	if t0, ok := getFloatValue(values, "T0_temp_mean"); ok {
		if t0 > 35.0 {
			faults = append(faults, "T0_HIGH")
		} else if t0 < -5.0 {
			faults = append(faults, "T0_LOW")
		}
	}
	
	// Check valve speeds
	if hotValve, ok := getFloatValue(values, "Hot_valve_speed"); ok {
		if hotValve > 0 && hotValve < 5.0 {
			faults = append(faults, "HOT_VALVE_MIN")
		}
	}
	
	if ahtValve, ok := getFloatValue(values, "AHT_valve_speed"); ok {
		if ahtValve > 0 && ahtValve < 5.0 {
			faults = append(faults, "AHT_VALVE_MIN")
		}
	}
	
	if len(faults) > 0 {
		return strings.Join(faults, ",")
	}
	
	return ""
}

// detectEModelFaults detects faults for E model machines
func detectEModelFaults(values map[string]interface{}) string {
	faults := []string{}
	
	// E models don't have FAULT_CODE column by default, but we can detect issues
	if lp, ok := getFloatValue(values, "LP_value"); ok {
		if lp < 2.0 {
			faults = append(faults, "LP_LOW_E")
		} else if lp > 5.5 {
			faults = append(faults, "LP_HIGH_E")
		}
	}
	
	if hp, ok := getFloatValue(values, "HP_value"); ok {
		if hp < 15.0 {
			faults = append(faults, "HP_LOW_E")
		} else if hp > 28.0 {
			faults = append(faults, "HP_HIGH_E")
		}
	}
	
	// Check heater operation
	if heater, ok := getFloatValue(values, "Heater_speed"); ok {
		if heater > 0 && heater < 10.0 {
			faults = append(faults, "HEATER_LOW")
		}
	}
	
	if len(faults) > 0 {
		return strings.Join(faults, ",")
	}
	
	return ""
}

// detectEPModelFaults detects faults for EP model machines (German)
func detectEPModelFaults(values map[string]interface{}) string {
	faults := []string{}
	
	// EP models have Fault_Code1 column
	if faultCode1, ok := getStringValue(values, "Fault_Code1"); ok && faultCode1 != "" {
		return faultCode1
	}
	
	// If Fault_Code1 is empty, detect from other parameters
	if lp, ok := getFloatValue(values, "LP"); ok {
		if lp < 1.8 {
			faults = append(faults, "LP_EP_LOW")
		} else if lp > 4.8 {
			faults = append(faults, "LP_EP_HIGH")
		}
	}
	
	if hp, ok := getFloatValue(values, "HP"); ok {
		if hp < 14.0 {
			faults = append(faults, "HP_EP_LOW")
		} else if hp > 26.0 {
			faults = append(faults, "HP_EP_HIGH")
		}
	}
	
	// Check temperatures with German machine column names
	if outletTemp, ok := getFloatValue(values, "AIR_OUTLET_TEMP"); ok {
		if outletTemp > 42.0 {
			faults = append(faults, "OUTLET_HIGH")
		} else if outletTemp < -8.0 {
			faults = append(faults, "OUTLET_LOW")
		}
	}
	
	if len(faults) > 0 {
		return strings.Join(faults, ",")
	}
	
	return ""
}

// detectGTPL124Faults detects faults for GTPL-124 machines
func detectGTPL124Faults(values map[string]interface{}) string {
	faults := []string{}
	
	// Similar to T model but no CR_valve columns
	if lp, ok := getFloatValue(values, "LP_value"); ok {
		if lp < 1.5 {
			faults = append(faults, "LP_LOW_124")
		} else if lp > 4.5 {
			faults = append(faults, "LP_HIGH_124")
		}
	}
	
	// Check if auto mode is on but not maintaining temperature
	if autoMode, ok := getIntValue(values, "Auto_mode"); ok && autoMode == 1 {
		if t0, ok := getFloatValue(values, "T0_temp_mean"); ok {
			if t0Set, ok := getFloatValue(values, "T0_set_point"); ok {
				diff := t0 - t0Set
				if diff > 5.0 {
					faults = append(faults, "TEMP_ABOVE_SET")
				} else if diff < -5.0 {
					faults = append(faults, "TEMP_BELOW_SET")
				}
			}
		}
	}
	
	if len(faults) > 0 {
		return strings.Join(faults, ",")
	}
	
	return ""
}

// detectThailandTFaults detects faults for Thailand T model machines
func detectThailandTFaults(values map[string]interface{}) string {
	faults := []string{}
	
	// Check condenser fan
	if condFan, ok := getFloatValue(values, "Cond_fan_speed"); ok {
		if condFan > 0 && condFan < 15.0 {
			faults = append(faults, "COND_FAN_LOW")
		}
	}
	
	// Similar to GTPL-124 with additional checks
	return detectGTPL124Faults(values)
}

// detectGTPL118Faults detects faults for GTPL-118 machines
func detectGTPL118Faults(values map[string]interface{}) string {
	faults := []string{}
	
	// Check condenser fan speed
	if condFan, ok := getFloatValue(values, "Condenser_fan_speed"); ok {
		if condFan > 0 && condFan < 20.0 {
			faults = append(faults, "COND_FAN_118_LOW")
		}
	}
	
	// Check AHT valve (note the spelling: AHT_vale_speed)
	if ahtValve, ok := getFloatValue(values, "AHT_vale_speed"); ok {
		if ahtValve > 0 && ahtValve < 10.0 {
			faults = append(faults, "AHT_VALVE_118_LOW")
		}
	}
	
	// Check T1 set point vs actual
	if t1, ok := getFloatValue(values, "T1_temp_mean"); ok {
		if t1Set, ok := getFloatValue(values, "T1_set_point"); ok {
			diff := t1 - t1Set
			if diff > 4.0 {
				faults = append(faults, "T1_ABOVE_SET_118")
			} else if diff < -4.0 {
				faults = append(faults, "T1_BELOW_SET_118")
			}
		}
	}
	
	if len(faults) > 0 {
		return strings.Join(faults, ",")
	}
	
	return ""
}

// detectGenericFaults detects generic faults for unknown machine types
func detectGenericFaults(values map[string]interface{}) string {
	faults := []string{}
	
	// Try to get common pressure values with different column names
	pressureKeys := []string{"LP_value", "LP", "Low_Pressure", "low_pressure", "pressure_low"}
	for _, key := range pressureKeys {
		if val, ok := getFloatValue(values, key); ok {
			if val < 1.0 {
				faults = append(faults, "PRESSURE_LOW")
			} else if val > 6.0 {
				faults = append(faults, "PRESSURE_HIGH")
			}
			break
		}
	}
	
	// Check for temperature issues
	tempKeys := []string{"T1_temp_mean", "T1", "temp1", "temperature", "Temp"}
	for _, key := range tempKeys {
		if val, ok := getFloatValue(values, key); ok {
			if val > 50.0 {
				faults = append(faults, "TEMP_HIGH")
			} else if val < -15.0 {
				faults = append(faults, "TEMP_LOW")
			}
			break
		}
	}
	
	if len(faults) > 0 {
		return strings.Join(faults, ",")
	}
	
	return ""
}

// Helper functions to extract values from interface{}
func getFloatValue(values map[string]interface{}, key string) (float64, bool) {
	if val, ok := values[key]; ok {
		switch v := val.(type) {
		case float64:
			return v, true
		case float32:
			return float64(v), true
		case int:
			return float64(v), true
		case int64:
			return float64(v), true
		case string:
			if f, err := parseFloat(v); err == nil {
				return f, true
			}
		case []byte:
			if f, err := parseFloat(string(v)); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

func getIntValue(values map[string]interface{}, key string) (int, bool) {
	if val, ok := values[key]; ok {
		switch v := val.(type) {
		case int:
			return v, true
		case int64:
			return int(v), true
		case float64:
			return int(v), true
		case string:
			if i, err := parseInt(v); err == nil {
				return i, true
			}
		case []byte:
			if i, err := parseInt(string(v)); err == nil {
				return i, true
			}
		}
	}
	return 0, false
}

func getStringValue(values map[string]interface{}, key string) (string, bool) {
	if val, ok := values[key]; ok {
		switch v := val.(type) {
		case string:
			return v, true
		case []byte:
			return string(v), true
		default:
			return fmt.Sprintf("%v", v), true
		}
	}
	return "", false
}

func parseFloat(s string) (float64, error) {
	// Remove any non-numeric characters except decimal point and minus
	clean := strings.TrimSpace(s)
	var result strings.Builder
	for _, r := range clean {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' {
			result.WriteRune(r)
		}
	}
	if result.Len() == 0 {
		return 0, fmt.Errorf("no numeric characters")
	}
	
	var f float64
	_, err := fmt.Sscanf(result.String(), "%f", &f)
	return f, err
}

func parseInt(s string) (int, error) {
	clean := strings.TrimSpace(s)
	var result strings.Builder
	for _, r := range clean {
		if r >= '0' && r <= '9' || r == '-' {
			result.WriteRune(r)
		}
	}
	if result.Len() == 0 {
		return 0, fmt.Errorf("no numeric characters")
	}
	
	var i int
	_, err := fmt.Sscanf(result.String(), "%d", &i)
	return i, err
}