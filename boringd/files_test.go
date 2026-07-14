package main

import (
	"strings"
	"testing"
)

func TestTransferCommandsUseSupportedGuestRuntime(t *testing.T) {
	commands := []string{
		uploadCommand(47001, "/root/input file.bin"),
		downloadCommand(47002, "/tmp/output file.bin"),
	}

	for _, command := range commands {
		if !strings.Contains(command, "python3 -c") {
			t.Fatalf("transfer command does not use Python: %s", command)
		}
		if strings.Contains(command, "node -e") {
			t.Fatalf("transfer command requires Node.js: %s", command)
		}
	}
}
