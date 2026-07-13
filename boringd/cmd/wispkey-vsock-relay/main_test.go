package main

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateLoopbackUpstream(t *testing.T) {
	for _, address := range []string{"127.0.0.1:7700", "[::1]:7700"} {
		if err := validateLoopbackUpstream(address); err != nil {
			t.Fatalf("validateLoopbackUpstream(%q) error = %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:7700", "192.0.2.1:7700", "localhost:7700", "missing-port"} {
		if err := validateLoopbackUpstream(address); err == nil {
			t.Fatalf("validateLoopbackUpstream(%q) unexpectedly succeeded", address)
		}
	}
}

func TestPrepareSocketPathRefusesRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "listener")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareSocketPath(path); err == nil {
		t.Fatal("prepareSocketPath() unexpectedly replaced a regular file")
	}
}

func TestProxyCopiesBothDirections(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	upstreamDone := make(chan error, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			upstreamDone <- acceptErr
			return
		}
		defer connection.Close()
		request := make([]byte, 4)
		if _, readErr := io.ReadFull(connection, request); readErr != nil {
			upstreamDone <- readErr
			return
		}
		if string(request) != "ping" {
			upstreamDone <- &unexpectedPayload{got: string(request)}
			return
		}
		_, writeErr := connection.Write([]byte("pong"))
		upstreamDone <- writeErr
	}()

	guest, relay := net.Pipe()
	defer guest.Close()
	go proxy(relay, listener.Addr().String())
	if _, err := guest.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 4)
	if _, err := io.ReadFull(guest, response); err != nil {
		t.Fatal(err)
	}
	if string(response) != "pong" {
		t.Fatalf("response = %q, want pong", response)
	}
	select {
	case err := <-upstreamDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("upstream proxy timed out")
	}
}

type unexpectedPayload struct {
	got string
}

func (e *unexpectedPayload) Error() string { return "unexpected payload: " + e.got }
