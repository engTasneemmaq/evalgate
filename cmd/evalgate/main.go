package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"evalgate/fixtures"
	"evalgate/internal/gate"
)

var versionIDs = []string{"support-v1", "support-v2", "support-v3"}

const demoSessionCookie = "evalgate_demo_session"

func readJSON[T any](name string) (T, error) {
	var value T
	var data []byte
	var err error
	if name == "" {
		return value, errors.New("missing input path")
	}
	if name[0] == '@' {
		data, err = fs.ReadFile(fixtures.Files, name[1:])
	} else {
		data, err = os.ReadFile(name)
	}
	if err == nil {
		err = json.Unmarshal(data, &value)
	}
	if err != nil {
		return value, fmt.Errorf("%s: %w", name, err)
	}
	return value, nil
}

func compare(args []string) int {
	flags := flag.NewFlagSet("compare", flag.ContinueOnError)
	suitePath := flags.String("suite", "@suite.json", "Test suite JSON")
	basePath := flags.String("baseline", "@support-v1.json", "Baseline responses")
	candidatePath := flags.String("candidate", "@support-v2.json", "Candidate responses")
	if flags.Parse(args) != nil {
		return 2
	}
	run, err := evaluate(context.Background(), *suitePath, *basePath, *candidatePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(run); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if run.Status == "Blocked" {
		return 1
	}
	return 0
}

func evaluate(ctx context.Context, suitePath, basePath, candidatePath string) (gate.Run, error) {
	suite, err := readJSON[gate.Suite](suitePath)
	if err != nil {
		return gate.Run{}, err
	}
	baseline, err := readJSON[gate.Version](basePath)
	if err != nil {
		return gate.Run{}, err
	}
	candidate, err := readJSON[gate.Version](candidatePath)
	if err != nil {
		return gate.Run{}, err
	}
	return compareVersions(ctx, suite, baseline, candidate)
}

func compareVersions(ctx context.Context, suite gate.Suite, baseline, candidate gate.Version) (gate.Run, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	run, err := gate.Compare(ctx, suite, baseline, candidate)
	if err == nil {
		var id [12]byte
		rand.Read(id[:])
		run.ID = hex.EncodeToString(id[:])
	}
	return run, err
}

func send(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Print(err)
	}
}

func problem(w http.ResponseWriter, status int, message string) {
	send(w, status, map[string]string{"error": message})
}

func decode(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("expected a single JSON object")
	}
	return nil
}

func versionPath(id string) string {
	for _, known := range versionIDs {
		if known == id {
			return "@" + known + ".json"
		}
	}
	return ""
}

func visitorSession(w http.ResponseWriter, r *http.Request) (string, error) {
	if cookie, err := r.Cookie(demoSessionCookie); err == nil && len(cookie.Value) == 64 {
		if _, err = hex.DecodeString(cookie.Value); err == nil {
			return cookie.Value, nil
		}
	}
	var id [32]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	value := hex.EncodeToString(id[:])
	http.SetCookie(w, &http.Cookie{
		Name:     demoSessionCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		SameSite: http.SameSiteLaxMode,
	})
	return value, nil
}

func routes(store *gate.Store, demo *gate.DemoStore, assets string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		if demo != nil {
			if _, err := visitorSession(w, r); err != nil {
				problem(w, 500, "Could not start a demo session.")
				return
			}
		}
		send(w, 200, map[string]any{
			"versions":         versionIDs,
			"publicDemo":       demo != nil,
			"importsEnabled":   demo == nil,
			"historyEphemeral": demo != nil,
		})
	})
	mux.HandleFunc("GET /api/runs", func(w http.ResponseWriter, r *http.Request) {
		var runs []gate.Run
		var err error
		if demo != nil {
			var sessionID string
			sessionID, err = visitorSession(w, r)
			if err == nil {
				runs, err = demo.List(r.Context(), sessionID)
			}
		} else {
			runs, err = store.List(r.Context())
		}
		if err != nil {
			log.Print(err)
			problem(w, 500, "Could not load runs.")
			return
		}
		send(w, 200, runs)
	})
	mux.HandleFunc("POST /api/runs", func(w http.ResponseWriter, r *http.Request) {
		var input struct{ Baseline, Candidate string }
		if decode(w, r, &input) != nil || input.Baseline == input.Candidate ||
			versionPath(input.Baseline) == "" || versionPath(input.Candidate) == "" {
			problem(w, 400, "Select two different response versions.")
			return
		}
		run, err := evaluate(r.Context(), "@suite.json", versionPath(input.Baseline), versionPath(input.Candidate))
		if err == nil && demo != nil {
			var sessionID string
			sessionID, err = visitorSession(w, r)
			if err == nil {
				err = demo.Add(r.Context(), sessionID, run)
			}
		} else if err == nil {
			err = store.Add(r.Context(), run)
		}
		if err != nil {
			log.Print(err)
			problem(w, 500, "Could not complete the run.")
			return
		}
		send(w, 201, run)
	})
	mux.HandleFunc("POST /api/import", func(w http.ResponseWriter, r *http.Request) {
		if demo != nil {
			problem(w, http.StatusForbidden, "Custom JSON imports are disabled in the public demo.")
			return
		}
		var input struct {
			Suite     gate.Suite
			Baseline  gate.Version
			Candidate gate.Version
		}
		if decode(w, r, &input) != nil || input.Baseline.ID == input.Candidate.ID {
			problem(w, 400, "Supply a suite and two different versions as JSON (512 KiB total).")
			return
		}
		run, err := compareVersions(r.Context(), input.Suite, input.Baseline, input.Candidate)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		if err = store.Add(r.Context(), run); err != nil {
			log.Print(err)
			problem(w, 500, "Could not save the imported run.")
			return
		}
		send(w, 201, run)
	})
	mux.HandleFunc("PATCH /api/runs/{id}/cases/{case}", func(w http.ResponseWriter, r *http.Request) {
		var input struct{ Decision string }
		if decode(w, r, &input) != nil || (input.Decision != "approved" && input.Decision != "rejected") {
			problem(w, 400, "Choose approved or rejected.")
			return
		}
		var run gate.Run
		var err error
		if demo != nil {
			var sessionID string
			sessionID, err = visitorSession(w, r)
			if err == nil {
				run, err = demo.Review(r.Context(), sessionID, r.PathValue("id"), r.PathValue("case"), input.Decision)
			}
		} else {
			run, err = store.Review(r.Context(), r.PathValue("id"), r.PathValue("case"), input.Decision)
		}
		switch {
		case errors.Is(err, gate.ErrNotFound):
			problem(w, 404, "This case is unavailable for review.")
		case err != nil:
			log.Print(err)
			problem(w, 500, "Could not save your review.")
		default:
			send(w, 200, run)
		}
	})
	mux.Handle("/", http.FileServer(http.Dir(assets)))
	return mux
}

func serve(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	defaultAddress := "127.0.0.1:8080"
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		defaultAddress = "0.0.0.0:" + port
	}
	address := flags.String("addr", defaultAddress, "HTTP listen address")
	assets := flags.String("assets", "web/dist", "Built frontend directory")
	data := flags.String("data", "data/runs.json", "Local run history path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	publicDemo := strings.EqualFold(os.Getenv("EVALGATE_PUBLIC_DEMO"), "true") || os.Getenv("EVALGATE_PUBLIC_DEMO") == "1"
	var store *gate.Store
	var demo *gate.DemoStore
	var err error
	if publicDemo {
		demo = gate.NewDemoStore(200, 20, 24*time.Hour)
	} else {
		store, err = gate.OpenStore(ctx, os.Getenv("DATABASE_URL"), *data)
		if err != nil {
			return err
		}
		defer store.Close()
	}
	server := &http.Server{Addr: *address, Handler: routes(store, demo, *assets),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	log.Printf("EvalGate listening at http://%s", *address)
	return server.ListenAndServe()
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "compare":
			os.Exit(compare(os.Args[2:]))
		case "serve":
			log.Fatal(serve(os.Args[2:]))
		}
	}
	fmt.Fprintln(os.Stderr, "Usage: evalgate serve | evalgate compare")
	os.Exit(2)
}
