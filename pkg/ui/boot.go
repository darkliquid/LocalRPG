package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"go.hasen.dev/shirei"
	app "go.hasen.dev/shirei/app"
)

// Run opens the native window and enters the shirei frame loop. It does not
// return; quit paths exit the process. SetupWindow must be called first, which
// Run does, so callers only supply the title and content size.
func Run(title string, w, h int, view shirei.FrameFn) {
	quietGPUFallback()
	app.SetupWindow(title, w, h)
	app.Run(view)
}

// gpuFallbackNoise is the notice shirei prints when its Linux GPU path cannot
// turn the compositor-advertised dmabuf modifier into a render target (seen on
// NVIDIA proprietary drivers and occasionally on multi-GPU setups). Shirei then
// uses its software renderer and the app runs normally, so the line is filtered
// to keep the log clean; every other message passes through untouched.
const gpuFallbackNoise = "gpurender: fallback to software: framebuffer incomplete"

// gpuQuietWindow bounds how long stderr is filtered. The GPU probe runs during
// startup, so a short window is enough and keeps normal output (including a
// startup panic) unfiltered.
const gpuQuietWindow = 3 * time.Second

// quietGPUFallback drops gpuFallbackNoise from stderr for the startup window.
func quietGPUFallback() {
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		return
	}
	os.Stderr = w
	go filterNoise(r, orig, gpuFallbackNoise)
	go func() {
		time.Sleep(gpuQuietWindow)
		os.Stderr = orig
		_ = w.Close()
	}()
}

// filterNoise copies lines from r to dst, dropping lines containing noise.
func filterNoise(r io.Reader, dst io.Writer, noise string) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if strings.Contains(sc.Text(), noise) {
			continue
		}
		fmt.Fprintln(dst, sc.Text())
	}
}
