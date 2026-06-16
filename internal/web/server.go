package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bobyasasas/kai-systemctl/internal/systemd"
)

type Server struct {
	manager *systemd.Manager
	mux     *http.ServeMux
}

type unitPayload struct {
	Name        string `json:"name"`
	Content     string `json:"content"`
	NewName     string `json:"newName"`
	Description string `json:"description"`
	ExecStart   string `json:"execStart"`
	WorkingDir  string `json:"workingDir"`
	User        string `json:"user"`
}

func NewServer(manager *systemd.Manager) http.Handler {
	s := &Server{
		manager: manager,
		mux:     http.NewServeMux(),
	}
	s.routes()
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/", s.index)
	s.mux.HandleFunc("/api/units", s.units)
	s.mux.HandleFunc("/api/units/", s.unit)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

func (s *Server) units(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		units, err := s.manager.List()
		writeJSON(w, units, err)
	case http.MethodPost:
		var p unitPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		content := p.Content
		if strings.TrimSpace(content) == "" {
			content = systemd.RenderService(systemd.ServiceTemplate{
				Description: p.Description,
				ExecStart:   p.ExecStart,
				WorkingDir:  p.WorkingDir,
				User:        p.User,
			})
		}
		unit, err := s.manager.Create(p.Name, content)
		writeJSON(w, unit, err)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) unit(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/units/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	name := parts[0]
	if len(parts) == 2 && parts[1] == "action" {
		s.action(w, r, name)
		return
	}

	switch r.Method {
	case http.MethodGet:
		content, err := s.manager.Read(name)
		writeJSON(w, map[string]string{"content": content}, err)
	case http.MethodPut:
		var p unitPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(p.NewName) != "" {
			unit, err := s.manager.Rename(name, p.NewName)
			writeJSON(w, unit, err)
			return
		}
		unit, err := s.manager.Update(name, p.Content)
		writeJSON(w, unit, err)
	case http.MethodDelete:
		err := s.manager.Delete(name)
		writeJSON(w, map[string]bool{"ok": err == nil}, err)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) action(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var p struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	err := s.manager.Systemctl(p.Action, name)
	writeJSON(w, map[string]bool{"ok": err == nil}, err)
}

func writeJSON(w http.ResponseWriter, v any, err error) {
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
