package avatar

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type UploadHandler struct {
	pipeline *Pipeline
}

func NewUploadHandler(pipeline *Pipeline) *UploadHandler {
	return &UploadHandler{pipeline: pipeline}
}

func (h *UploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart request"})
		return
	}

	file, header, err := r.FormFile("avatar")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "avatar is required"})
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read avatar"})
		return
	}

	input := AvatarRequest{
		UserID: r.FormValue("user_id"), BuildID: r.FormValue("build_id"),
		ReleaseID: r.FormValue("release_id"), OperationID: r.FormValue("operation_id"),
		Filename: header.Filename, File: content,
	}
	if input.UserID == "" || input.BuildID == "" || input.ReleaseID == "" || input.OperationID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id, build_id, release_id, and operation_id are required"})
		return
	}

	result, runErr := h.pipeline.Run(r.Context(), input)
	if runErr != nil {
		status := http.StatusBadGateway
		var apiErr *APIError
		if errors.As(runErr, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
			status = apiErr.HTTPStatus
		}
		writeJSON(w, status, map[string]any{"error": runErr.Error(), "pipeline": result})
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
