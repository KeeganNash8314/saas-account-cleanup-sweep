package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/example/saas-cleanup-sweep/cleanup"
	"github.com/example/saas-cleanup-sweep/infrai"
)

type sweepRequest struct {
	Now     time.Time        `json:"now"`
	Tenants []cleanup.Tenant `json:"tenants"`
}

type sweepResponse struct {
	Decisions []cleanup.Decision `json:"decisions"`
	Deleted   []string           `json:"deleted"`
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: sweepd serve|register")
	}
	switch os.Args[1] {
	case "serve":
		serve()
	case "register":
		if err := register(context.Background()); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown command %q", os.Args[1])
	}
}

func serve() {
	days := 30
	if raw := os.Getenv("STALE_AFTER_DAYS"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			log.Fatal("STALE_AFTER_DAYS must be a positive integer")
		}
		days = parsed
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/sweep", sweepHandler(time.Duration(days)*24*time.Hour))
	address := ":8080"
	if value := os.Getenv("LISTEN_ADDR"); value != "" {
		address = value
	}
	log.Printf("sweep admin listening on %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}

func sweepHandler(staleAfter time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input sweepRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "invalid sweep input", http.StatusBadRequest)
			return
		}
		if input.Now.IsZero() {
			input.Now = time.Now().UTC()
		}
		decisions := cleanup.Sweep(input.Tenants, input.Now, staleAfter)
		result := sweepResponse{Decisions: decisions, Deleted: make([]string, 0)}
		for _, decision := range decisions {
			if decision.Delete {
				result.Deleted = append(result.Deleted, decision.TenantID)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

func register(ctx context.Context) error {
	key := os.Getenv("INFRAI_API_KEY")
	task := os.Getenv("SWEEP_TASK_URL")
	if key == "" || task == "" {
		return errors.New("INFRAI_API_KEY and SWEEP_TASK_URL are required")
	}
	client := infrai.NewCronClient(key, nil)
	jobID, err := client.CreateCron(ctx, "15 2 * * *", task)
	if err != nil {
		var apiErr *infrai.InfraiError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			return fmt.Errorf("cron registration rejected: %w", apiErr)
		}
		return err
	}
	fmt.Printf("registered cleanup job %s\n", jobID)
	return nil
}
