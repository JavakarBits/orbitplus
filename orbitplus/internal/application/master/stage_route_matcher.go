package master

import "encoding/json"

func stageContainsRoute(stage []byte, fromCode, toCode string) bool {
	var value any
	if err := json.Unmarshal(stage, &value); err != nil {
		return false
	}
	codes := stationCodesFromValue(value)
	fromIndex := indexOfStation(codes, fromCode)
	if fromIndex < 0 {
		return false
	}
	for _, code := range codes[fromIndex+1:] {
		if code == toCode {
			return true
		}
	}
	return false
}

func stationCodesFromValue(value any) []string {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	for _, field := range []string{"stationPoint", "stationPoints", "stations"} {
		if points, ok := object[field].([]any); ok {
			codes := make([]string, 0, len(points))
			for _, point := range points {
				if code := stationCode(point); code != "" {
					codes = append(codes, code)
				}
			}
			if len(codes) > 0 {
				return codes
			}
		}
	}
	codes := make([]string, 0, 2)
	if code := stationCode(object["fromStation"]); code != "" {
		codes = append(codes, code)
	}
	if code := stationCode(object["toStation"]); code != "" {
		codes = append(codes, code)
	}
	return codes
}

func stationCode(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	for _, field := range []string{"stationCode", "code"} {
		if code, ok := object[field].(string); ok {
			return code
		}
	}
	for _, field := range []string{"station", "stationDetails"} {
		if code := stationCode(object[field]); code != "" {
			return code
		}
	}
	return ""
}

func indexOfStation(codes []string, wanted string) int {
	for index, code := range codes {
		if code == wanted {
			return index
		}
	}
	return -1
}
