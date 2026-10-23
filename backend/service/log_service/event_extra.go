package log_service

import "encoding/json"

func marshalExtra(extra map[string]any) string {
	if len(extra) == 0 {
		return ""
	}
	byteData, err := json.Marshal(extra)
	if err != nil {
		return ""
	}
	return string(byteData)
}
