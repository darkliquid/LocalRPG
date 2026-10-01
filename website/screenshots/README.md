# Showcase screenshots

The images the documentation site displays in its gallery. They are committed
here rather than generated during deployment, so a Pages build never depends on
a browser.

They are JPEGs: they are photographs of a running UI, and a lossless PNG of a
full-window screenshot with artwork in it runs to well over a megabyte for no
visible gain.

`mise run site:screenshots` regenerates the first four: it launches the app
against a throwaway copy of `website/demo`, drives it headlessly through
`website/scenarios/screenshots.yaml`, and writes the files here. The scenario
names `.jpg` paths, and the driver re-encodes Chrome's PNG capture as JPEG
(quality 88, or per-step `quality`), so the extension and the bytes agree. The
next `mise run site:build` picks the images up automatically.

## Expected files

The gallery reads these names. Any that are missing renders as a placeholder
frame rather than disappearing, so the site is presentable before the first
capture.

| File                      | Shows                                 | Captured by |
| ------------------------- | ------------------------------------- | ----------- |
| `01-launcher.jpg`         | Launcher Hub with a campaign loaded   | scenario    |
| `02-worlds-studio.jpg`    | Worlds Studio                         | scenario    |
| `03-systems-studio.jpg`   | Systems Studio                        | scenario    |
| `04-settings.jpg`         | Settings Studio                       | scenario    |
| `05-codex.jpg`            | Codex drawer over a campaign          | hand        |
| `06-story-theatre.jpg`    | Story Theatre playing a turn          | hand        |

The last two need a campaign open in the theatre, which the headless scenario
cannot reach on its own. Capture them by hand and drop them in with the names
above. They are not cropped to 16:10: the app's own window is the frame, and the
gallery shows each image whole rather than trimming UI off the edges.
