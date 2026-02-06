package api

import (
	"net/http"
	"os"
	"github.com/gorilla/mux"
)



func AuthMiddleWare(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r*http.Request) {
		header := r.Header.Get("Authorization")
		if header != "Bearer "+token {
			http.Error(w, "Unauthorized Action", http.StatusUnauthorized)
			return 
		}
		next(w, r)
	}
}

func NewRouter(h *Handlers) *mux.Router {
	r := mux.NewRouter()

	r.HandleFunc("/health", h.HealthCheck).Methods("GET")
	r.HandleFunc("/jobs", h.CreateJob).Methods("POST")
	r.HandleFunc("/jobs", h.ListJobs).Methods("GET")
	r.HandleFunc("/endjobs/{id}", h.CancelJob).Methods("POST")
	r.HandleFunc("/jobs/{id}", h.GetJob).Methods("GET")
	r.HandleFunc("/server", AuthMiddleWare(os.Getenv("SHUTDOWN_TOKEN"), func(w http.ResponseWriter, r *http.Request) {
    (h.QuitFunction)()
		w.WriteHeader(http.StatusOK)
	})).Methods("DELETE")

	return r
}
