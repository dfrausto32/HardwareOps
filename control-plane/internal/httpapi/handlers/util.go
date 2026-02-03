package handlers

import "encoding/json"

func isJSONObject(data []byte) bool {
	var obj map[string]interface{}
	return json.Unmarshal(data, &obj) == nil
}
