package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"strings"
	"time"
)

// File transfer. The serial console can't move bulk data (the guest tty input
// buffer overflows past a few KB), so instead we send a short command over the
// console to spin up a one-shot Python TCP helper in the guest, then stream the
// file over the guest network (host->guest works via the bridge). Python ships
// in every supported image. Requires a connected machine (needs an IP).

const fileSizeCap = 16 << 20 // 16 MiB

func sanitizeName(name string) string {
	name = path.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, name)
	if name == "" || name == "." || name == ".." {
		return "upload.bin"
	}
	return name
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// xferPort picks a per-request port so back-to-back transfers don't collide on a
// lingering listener.
func xferPort() int { return 47000 + int(time.Now().UnixNano()/1e6%2000) }

// uploadCommand starts a single-use guest receiver without requiring Node.js.
func uploadCommand(port int, destination string) string {
	return fmt.Sprintf(
		`python3 -c 'import shutil,socket,sys;s=socket.socket();s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);s.bind(("0.0.0.0",%d));s.listen(1);c,_=s.accept();f=open(sys.argv[1],"wb");shutil.copyfileobj(c.makefile("rb"),f);f.close();c.close();s.close()' %s 2>/dev/null &`+"\n",
		port, shellQuote(destination))
}

// downloadCommand starts a single-use guest sender without requiring Node.js.
func downloadCommand(port int, source string) string {
	return fmt.Sprintf(
		`python3 -c 'import shutil,socket,sys;f=open(sys.argv[1],"rb");s=socket.socket();s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);s.bind(("0.0.0.0",%d));s.listen(1);c,_=s.accept();o=c.makefile("wb");shutil.copyfileobj(f,o);o.close();f.close();c.close();s.close()' %s 2>/dev/null &`+"\n",
		port, shellQuote(source))
}

// dialGuest connects to the just-started guest helper, retrying while it boots.
func dialGuest(ip string, port int) (net.Conn, error) {
	addr := net.JoinHostPort(ip, fmt.Sprint(port))
	var last error
	for i := 0; i < 25; i++ {
		if c, err := net.DialTimeout("tcp", addr, 800*time.Millisecond); err == nil {
			return c, nil
		} else {
			last = err
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, last
}

func (s *Server) guestFor(id string) (*Console, string, bool) {
	console, ok := s.mgr.Console(id)
	if !ok {
		return nil, "", false
	}
	ip, ok := s.mgr.machineIP(id)
	if !ok {
		return nil, "", false
	}
	return console, ip, true
}

// handleUpload streams a file into the guest's /root over the network.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > fileSizeCap {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "file too big (16 MiB max)"})
		return
	}
	console, ip, ok := s.guestFor(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "file transfer needs a connected computer (turn internet on / use a desktop)"})
		return
	}
	name := sanitizeName(r.Header.Get("X-Filename"))
	dest := "/root/" + name
	port := xferPort()
	cmd := uploadCommand(port, dest)
	if _, err := console.Write([]byte(cmd)); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "couldn't write to the computer's console"})
		return
	}

	conn, err := dialGuest(ip, port)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "couldn't reach the computer's uploader"})
		return
	}
	defer conn.Close()
	n, _ := io.Copy(conn, io.LimitReader(r.Body, fileSizeCap))
	if tcp, ok := conn.(*net.TCPConn); ok {
		tcp.CloseWrite() // EOF -> the receiver finishes writing and exits
	}
	io.Copy(io.Discard, conn) // wait for the receiver to close
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": dest, "bytes": n})
}

// handleDownload streams a file out of the guest over the network.
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "path required"})
		return
	}
	console, ip, ok := s.guestFor(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "file transfer needs a connected computer"})
		return
	}
	port := xferPort()
	cmd := downloadCommand(port, p)
	if _, err := console.Write([]byte(cmd)); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "couldn't write to the computer's console"})
		return
	}

	conn, err := dialGuest(ip, port)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "couldn't reach the computer's downloader"})
		return
	}
	defer conn.Close()
	// Peek the first byte: a missing file makes node exit before sending anything.
	first := make([]byte, 1)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	nr, _ := io.ReadFull(conn, first)
	if nr == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "no such file, or it's empty"})
		return
	}
	conn.SetReadDeadline(time.Time{})
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", sanitizeName(path.Base(p))))
	w.Write(first[:nr])
	io.Copy(w, conn)
}
