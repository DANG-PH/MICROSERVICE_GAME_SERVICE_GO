package http

import "net/http"

func NewRouter() *http.ServeMux {
	mux := http.NewServeMux()

	// TODO: register domain routes here
	// Example:
	// userHandler := user.NewHandler(userService)
	// mux.HandleFunc("GET /v1/users/{id}", userHandler.GetByID)

	return mux
}
