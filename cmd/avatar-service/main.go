package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/infrai-examples/devtools-avatar-pipeline/internal/avatar"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}

	client := avatar.NewClient(avatar.DefaultBaseURL, key, &http.Client{Timeout: 30 * time.Second})
	handler := avatar.NewUploadHandler(avatar.NewPipeline(client))

	mux := http.NewServeMux()
	mux.Handle("POST /avatars", handler)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("avatar service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
