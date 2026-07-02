package main

import (
	"net/http"
)

// handleScreenshot captures a single PNG framebuffer of a desktop machine.
func (s *Server) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	guest, err := s.mgr.DialVsock(id, VsockPort)
	if err != nil {
		if err == ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		} else {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		}
		return
	}
	defer guest.Close()

	cli, err := newRFBClient(guest)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "rfb: " + err.Error()})
		return
	}
	png, err := cli.Screenshot()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "screenshot: " + err.Error()})
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(png)
}

// handleAgent runs the computer-use loop (wired once BORING_ANTHROPIC_KEY is set).
func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	s.runAgent(w, r)
}
