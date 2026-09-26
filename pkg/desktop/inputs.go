package desktop

import (
	. "go.hasen.dev/shirei/widgets"
)

// Form fields do not steal focus when they first render. A focused text field
// animates its caret and requests a frame every frame for five seconds, so
// auto-focusing the first field makes every form redraw continuously (and
// flicker) each time a settings tab or studio is switched. Users click a field
// to edit it; nothing grabs focus behind their back.

// FieldInput is a single-line field that does not auto-focus.
func FieldInput(buf *string) {
	TextInputExt(buf, TextInputAttrs{NoAutoFocus: true})
}

// FieldArea is a multi-line field that does not auto-focus.
func FieldArea(buf *string) {
	attrs := DefaultMultilineTextInputAttrs()
	attrs.NoAutoFocus = true
	TextInputExt(buf, attrs)
}

// FieldPassword is a masked field that does not auto-focus.
func FieldPassword(buf *string) {
	attrs := DefaultTextInputAttrs()
	attrs.Masked = true
	attrs.NoAutoFocus = true
	TextInputExt(buf, attrs)
}

// FieldDirectory is a directory browser whose text field does not auto-focus.
func FieldDirectory(buf *string) {
	attrs := DefaultFileBrowserAttrs()
	attrs.NoAutoFocus = true
	DirectoryBrowseExt(buf, attrs)
}
