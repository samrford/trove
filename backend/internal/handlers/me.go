package handlers

import (
	"encoding/json"
	"net/http"
)

// HandleMe returns the authenticated user's id and email.
func HandleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"id":    GetUserID(r.Context()),
		"email": GetUserEmail(r.Context()),
	})
}
