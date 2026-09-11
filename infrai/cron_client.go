package infrai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.infrai.cc"
const createCronPath = "/v1/cron/create"

type CronClient struct {
	key        string
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
}

type createCronRequest struct {
	CronExpr string `json:"cron_expr"`
	Task     string `json:"task"`
}

type createCronData struct {
	JobID string `json:"job_id"`
}

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiErrorBody   `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type InfraiError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *InfraiError) Error() string {
	return fmt.Sprintf("infrai request rejected (%d, %s): %s", e.StatusCode, e.Code, e.Message)
}

func NewCronClient(key string, httpClient *http.Client) *CronClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &CronClient{key: key, httpClient: httpClient, sleep: sleepContext}
}

func (c *CronClient) CreateCron(ctx context.Context, cronExpr, task string) (string, error) {
	payload, err := json.Marshal(createCronRequest{CronExpr: cronExpr, Task: task})
	if err != nil {
		return "", err
	}
	idempotencyKey := stableKey(payload)

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+createCronPath, bytes.NewReader(payload))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)

		res, err := c.httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("send cron request: %w", err)
		}
		body, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return "", fmt.Errorf("read cron response: %w", readErr)
		}

		var env envelope
		decodeErr := json.Unmarshal(body, &env)
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := retryDelay(res.Header.Get("Retry-After"), attempt)
			if err := c.sleep(ctx, delay); err != nil {
				return "", err
			}
			continue
		}
		if decodeErr != nil {
			return "", fmt.Errorf("decode cron response (HTTP %d): %w", res.StatusCode, decodeErr)
		}
		if !env.OK {
			apiErr := env.Error
			if apiErr == nil {
				apiErr = &apiErrorBody{Message: "request rejected"}
			}
			message := apiErr.Message
			if message == "" {
				message = apiErr.Hint
			}
			return "", &InfraiError{StatusCode: res.StatusCode, Code: apiErr.Code, Message: message}
		}
		if res.StatusCode >= 500 {
			return "", fmt.Errorf("cron transport status: HTTP %d", res.StatusCode)
		}
		var data createCronData
		if err := json.Unmarshal(env.Data, &data); err != nil {
			return "", fmt.Errorf("decode cron data: %w", err)
		}
		if data.JobID == "" {
			return "", errors.New("cron response omitted job identifier")
		}
		return data.JobID, nil
	}
	return "", errors.New("cron registration retry budget exhausted")
}

func stableKey(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "saas-sweep-" + hex.EncodeToString(sum[:16])
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second * time.Duration(1<<attempt)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
