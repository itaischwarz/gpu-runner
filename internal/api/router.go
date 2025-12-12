package api

import (
    "github.com/gorilla/mux"
)

func NewRouter(h *Handlers) *mux.Router {
    r := mux.NewRouter()

    r.HandleFunc("/jobs", h.CreateJob).Methods("POST")
    r.HandleFunc("/endjobs/{id}", h.CancelJob).Methods("POST")
    r.HandleFunc("/jobs/{id}", h.GetJob).Methods("GET")
    
    return r
}