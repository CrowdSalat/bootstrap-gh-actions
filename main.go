package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	hostname, _ := os.Hostname()
	fmt.Printf("Hello from %s/%s (host=%s)\n", runtime.GOOS, runtime.GOARCH, hostname)
}
