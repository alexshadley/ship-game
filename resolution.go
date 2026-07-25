package main

import (
	"strconv"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The game renders at two fixed logical resolutions — the 640×360 world texture
// and the 1920×1080 UI (windowWidth×windowHeight) the menu/designer lay out in —
// and both are scaled to fit the actual window each frame. currentViewport is the
// letterboxed on-screen rectangle they're drawn into; the mouse helpers invert
// that mapping so input lines up with whatever the player clicks.

// currentViewport returns the largest 16:9 rectangle that fits the drawing area,
// centered. Both logical targets share this aspect, so one viewport frames both.
//
// It measures the render (framebuffer) size, not the screen size: on a HiDPI
// display those differ (e.g. a 1360-point window backs a 2720-pixel framebuffer),
// and all drawing happens in framebuffer pixels. Using the screen size here would
// size the viewport to a fraction of the framebuffer and shove everything into a
// corner.
func currentViewport() rl.Rectangle {
	sw := float32(rl.GetRenderWidth())
	sh := float32(rl.GetRenderHeight())
	scale := sw / float32(windowWidth)
	if s := sh / float32(windowHeight); s < scale {
		scale = s
	}
	w := float32(windowWidth) * scale
	h := float32(windowHeight) * scale
	return rl.NewRectangle((sw-w)/2, (sh-h)/2, w, h)
}

// toLogical maps the OS mouse position into a logical frame of size (lw, lh),
// undoing the letterbox offset and scale of the current viewport. The mouse is
// reported in screen (point) space, so it's first scaled into render (pixel)
// space to match the viewport; on non-HiDPI displays that scale is 1.
func toLogical(p rl.Vector2, lw, lh float32) rl.Vector2 {
	vp := currentViewport()
	if vp.Width == 0 || vp.Height == 0 {
		return rl.NewVector2(0, 0)
	}
	sx := float32(rl.GetRenderWidth()) / float32(rl.GetScreenWidth())
	sy := float32(rl.GetRenderHeight()) / float32(rl.GetScreenHeight())
	return rl.NewVector2((p.X*sx-vp.X)/vp.Width*lw, (p.Y*sy-vp.Y)/vp.Height*lh)
}

// uiMouse is the OS mouse position in the menu/designer's 1920×1080 UI space.
func uiMouse() rl.Vector2 {
	return toLogical(rl.GetMousePosition(), float32(windowWidth), float32(windowHeight))
}

// parseResolution reads a "WxH" string (e.g. "1280x720") into pixel dimensions.
// It returns ok=false for anything it can't parse or non-positive sizes.
func parseResolution(s string) (w, h int32, ok bool) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	wv, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	hv, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || wv <= 0 || hv <= 0 {
		return 0, 0, false
	}
	return int32(wv), int32(hv), true
}

// fitToMonitor picks a 16:9 window that fits within ~90% of the monitor, so the
// default window never opens larger than the screen it lands on.
func fitToMonitor(monW, monH int32) (int32, int32) {
	maxW := float32(monW) * 0.9
	maxH := float32(monH) * 0.9
	w := maxW
	h := w * float32(windowHeight) / float32(windowWidth)
	if h > maxH {
		h = maxH
		w = h * float32(windowWidth) / float32(windowHeight)
	}
	return int32(w), int32(h)
}

// toggleFullscreen flips between fullscreen (at the monitor's native resolution)
// and the given windowed size. The viewport recomputes from the window size each
// frame, so letterbox bars appear automatically on non-16:9 displays.
func toggleFullscreen(windowedW, windowedH int32) {
	if rl.IsWindowFullscreen() {
		rl.ToggleFullscreen()
		rl.SetWindowSize(int(windowedW), int(windowedH))
	} else {
		mon := rl.GetCurrentMonitor()
		rl.SetWindowSize(int(rl.GetMonitorWidth(mon)), int(rl.GetMonitorHeight(mon)))
		rl.ToggleFullscreen()
	}
}
