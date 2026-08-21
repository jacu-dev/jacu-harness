package reportgen

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/report"
)

type ServeOptions struct {
	InputPath string
	Addr      string
	Ready     chan string
}

type DecisionWrite struct {
	ID     string `json:"id"`
	Answer string `json:"answer"`
}

func Serve(ctx context.Context, options ServeOptions) error {
	addr := strings.TrimSpace(options.Addr)
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host != "127.0.0.1" {
		return errors.New("serve binds 127.0.0.1 only")
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	if options.Ready != nil {
		options.Ready <- listener.Addr().String()
	}
	var mu sync.Mutex
	server := &http.Server{
		Addr:              listener.Addr().String(),
		ReadHeaderTimeout: 2 * time.Second,
		Handler:           serveMux(options.InputPath, &mu),
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	serveErr := server.Serve(listener)
	if errors.Is(serveErr, http.ErrServerClosed) || ctx.Err() != nil {
		return nil
	}
	return serveErr
}

func serveMux(path string, mu *sync.Mutex) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/" {
			http.NotFound(writer, request)
			return
		}
		mu.Lock()
		doc, err := Load(path)
		mu.Unlock()
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		body, err := HTML(doc)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(body))
	})
	mux.HandleFunc("/decision", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var write DecisionWrite
		if err := json.NewDecoder(request.Body).Decode(&write); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		doc, err := Load(path)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		if doc.Kind == report.KindAudit {
			http.Error(writer, "audit reports are read-only", http.StatusForbidden)
			return
		}
		found := false
		for index := range doc.Blocks.Decision {
			if doc.Blocks.Decision[index].ID == write.ID {
				doc.Blocks.Decision[index].Answer = write.Answer
				found = true
				break
			}
		}
		if !found {
			http.Error(writer, "unknown decision", http.StatusNotFound)
			return
		}
		encoded, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func Load(path string) (report.Report, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- path is the owner-supplied report JSON file
	if err != nil {
		return report.Report{}, err
	}
	var doc report.Report
	if err := json.Unmarshal(raw, &doc); err != nil {
		return report.Report{}, err
	}
	return doc, nil
}
