package main

import "github.com/egoist/mygo/ui"

// Icons drawn on a 24 grid with 1.6 strokes, in the color of the text.
func icon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

var (
	folderIcon = icon(`<path d="M3.5 7.2c0-1 .8-1.7 1.7-1.7h3.9l2.1 2.2h7.6c1 0 1.7.8 1.7 1.7v7.9c0 1-.8 1.7-1.7 1.7H5.2c-1 0-1.7-.8-1.7-1.7z"/><path d="M3.5 10.2h17"/>`)
	copyIcon   = icon(`<rect x="8.5" y="8.5" width="11" height="11" rx="2.2"/><path d="M15.5 8.5V6.2c0-1-.8-1.7-1.7-1.7H6.2c-1 0-1.7.8-1.7 1.7v7.6c0 1 .8 1.7 1.7 1.7h2.3"/>`)
	eyeIcon    = icon(`<path d="M2.8 12s3.3-6 9.2-6 9.2 6 9.2 6-3.3 6-9.2 6-9.2-6-9.2-6z"/><circle cx="12" cy="12" r="2.6"/>`)
	eyeOffIcon = icon(`<path d="M9.9 6.3A9.6 9.6 0 0 1 12 6c5.9 0 9.2 6 9.2 6a15.6 15.6 0 0 1-2.6 3.3M6.4 7.9C4 9.6 2.8 12 2.8 12s3.3 6 9.2 6c1.6 0 3-.4 4.2-1.1"/><path d="M10.2 10.2a2.6 2.6 0 0 0 3.6 3.6"/><path d="M4 4l16 16"/>`)
	logIcon    = icon(`<path d="M6.5 3.5h7.3l4.7 4.7v11c0 .7-.6 1.3-1.3 1.3H6.5c-.7 0-1.3-.6-1.3-1.3V4.8c0-.7.6-1.3 1.3-1.3z"/><path d="M13.5 3.5v5h5"/><path d="M8.5 13h7M8.5 16.5h5"/>`)
	alertIcon  = icon(`<circle cx="12" cy="12" r="8.8"/><path d="M12 7.8v5"/><path d="M12 16.2v.1" stroke-width="2.2"/>`)
)
