package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

func setupLog() {
	if dataDir == "" {
		return
	}
	_ = os.MkdirAll(dataDir, 0o755)
	f, err := os.OpenFile(filepath.Join(dataDir, "agent.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	w := io.MultiWriter(f)
	os.Stdout = f
	os.Stderr = f
	log.SetOutput(w)
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
