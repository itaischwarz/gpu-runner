package api

import (
    "github.com/gorilla/mux"
)

func NewRouter(h *Handlers) *mux.Router {
    r := mux.NewRouter()

    r.HandleFunc("/jobs", h.CreateJob).Methods("POST")
    r.HandleFunc("/end-jobs/{id}", h.Canc).Methods("POST")
    r.HandleFunc("/jobs/{id}", h.GetJob).Methods("GET")

    return r
}