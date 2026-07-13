package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

func main() {
	listenPath := flag.String("listen-unix", "", "Firecracker port socket path to bind")
	upstream := flag.String("upstream", "", "loopback WispKey or SSH-tunnel address")
	flag.Parse()

	if err := run(*listenPath, *upstream); err != nil {
		log.Fatal(err)
	}
}

func run(listenPath, upstream string) error {
	if listenPath == "" || !filepath.IsAbs(listenPath) {
		return errors.New("-listen-unix must be an absolute path")
	}
	if err := validateLoopbackUpstream(upstream); err != nil {
		return err
	}
	if err := prepareSocketPath(listenPath); err != nil {
		return err
	}

	listener, err := net.Listen("unix", listenPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()
	defer os.Remove(listenPath)
	if err := os.Chmod(listenPath, 0o600); err != nil {
		return fmt.Errorf("chmod listener: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		guest, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		go proxy(guest, upstream)
	}
}

func validateLoopbackUpstream(upstream string) error {
	host, _, err := net.SplitHostPort(upstream)
	if err != nil {
		return fmt.Errorf("invalid -upstream: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("-upstream must be a loopback IP")
	}
	return nil
}

func prepareSocketPath(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create socket directory: %w", err)
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect socket path: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return errors.New("refusing to replace a non-socket path")
	}
	return os.Remove(path)
}

func proxy(guest net.Conn, upstreamAddress string) {
	defer guest.Close()
	upstream, err := net.Dial("tcp", upstreamAddress)
	if err != nil {
		return
	}
	defer upstream.Close()

	done := make(chan struct{}, 2)
	copyStream := func(destination, source net.Conn) {
		_, _ = io.Copy(destination, source)
		if closer, ok := destination.(interface{ CloseWrite() error }); ok {
			_ = closer.CloseWrite()
		}
		done <- struct{}{}
	}
	go copyStream(upstream, guest)
	go copyStream(guest, upstream)
	<-done
}
