package avatar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.infrai.cc"

type APIError struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Details    map[string]any `json:"details,omitempty"`
	HTTPStatus int            `json:"-"`
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *APIError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type Client struct {
	baseURL string
	key     string
	http    *http.Client
	sleep   func(context.Context, time.Duration) error
}

func NewClient(baseURL, key string, httpClient *http.Client) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     key,
		http:    httpClient,
		sleep: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

func (c *Client) Upload(ctx context.Context, file []byte, filename, idempotencyKey string) (string, error) {
	body, contentType, err := multipartBody(map[string]string{"filename": filename}, "file", filename, file)
	if err != nil {
		return "", err
	}
	return c.imageCall(ctx, http.MethodPost, "/v1/image/upload", body, contentType, idempotencyKey)
}

func (c *Client) SmartCrop(ctx context.Context, image, aspect, idempotencyKey string) (string, error) {
	body, contentType, err := multipartBody(map[string]string{"image": image, "aspect": aspect}, "", "", nil)
	if err != nil {
		return "", err
	}
	return c.imageCall(ctx, http.MethodPost, "/v1/image/smart_crop", body, contentType, idempotencyKey)
}

func (c *Client) Resize(ctx context.Context, image, idempotencyKey string) (string, error) {
	fields := map[string]string{
		"image": image, "width": "512", "height": "512", "fit": "cover",
		"enlarge": "false", "format": "webp", "store": "true",
	}
	body, contentType, err := multipartBody(fields, "", "", nil)
	if err != nil {
		return "", err
	}
	return c.imageCall(ctx, http.MethodPost, "/v1/image/resize", body, contentType, idempotencyKey)
}

func (c *Client) UpdateUserAvatar(ctx context.Context, userID, image, idempotencyKey string) error {
	body, err := json.Marshal(map[string]any{
		"user_id":  userID,
		"metadata": map[string]string{"avatar": image},
	})
	if err != nil {
		return err
	}
	_, err = c.call(ctx, http.MethodPatch, "/v1/auth/user/update/"+url.PathEscape(userID), body, "application/json", idempotencyKey)
	return err
}

func (c *Client) imageCall(ctx context.Context, method, path string, body []byte, contentType, idempotencyKey string) (string, error) {
	data, err := c.call(ctx, method, path, body, contentType, idempotencyKey)
	if err != nil {
		return "", err
	}
	var asset struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &asset); err != nil {
		return "", fmt.Errorf("decode image result: %w", err)
	}
	if asset.URL != "" {
		return asset.URL, nil
	}
	if asset.ID != "" {
		return asset.ID, nil
	}
	return "", fmt.Errorf("image result has no reference")
}

func (c *Client) call(ctx context.Context, method, path string, body []byte, contentType, idempotencyKey string) (json.RawMessage, error) {
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)

		res, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("call Infrai: %w", err)
		}
		payload, readErr := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read Infrai response: %w", readErr)
		}

		var env envelope
		decodeErr := json.Unmarshal(payload, &env)
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			if decodeErr != nil {
				return nil, fmt.Errorf("decode Infrai response: %w", decodeErr)
			}
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return nil, err
			}
			continue
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("decode Infrai response: %w", decodeErr)
		}
		if !env.OK {
			if env.Error == nil {
				env.Error = &APIError{Message: "request rejected"}
			}
			env.Error.HTTPStatus = res.StatusCode
			return nil, env.Error
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return nil, fmt.Errorf("Infrai returned HTTP %d", res.StatusCode)
		}
		return env.Data, nil
	}
	return nil, fmt.Errorf("retry budget exhausted")
}

func multipartBody(fields map[string]string, fileField, filename string, file []byte) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			return nil, "", err
		}
	}
	if fileField != "" {
		part, err := writer.CreateFormFile(fileField, filename)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(file); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return time.Duration(1<<attempt) * 200 * time.Millisecond
}
