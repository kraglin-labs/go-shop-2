package handlers

import (
	"encoding/json"
	"net/http"

	"go-shop-2/internal/middleware"
)

func Me(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(claims)
}
