package handlers

import (
	"encoding/json"
	"net/http"
)

func AdminStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "admin access granted",
		"level":  "system",
	})
}
