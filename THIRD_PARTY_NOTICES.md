# Third-party notices

LocalRPG is released under the MIT License; see [LICENSE](LICENSE). It is built
from open-source libraries and uses typefaces and icons that carry their own
terms, so this file records both.

## Assets

| Asset          | Used by                                         | Licence      | Ships as                                  |
| -------------- | ----------------------------------------------- | ------------ | ----------------------------------------- |
| EB Garamond    | Interface prose, and the theatre's video frames | OFL-1.1      | CDN for the interface; embedded for video |
| Inter          | The theatre's video frames                      | OFL-1.1      | Embedded in the binary                    |
| JetBrains Mono | Code, in the interface and on video             | OFL-1.1      | CDN for the interface; embedded for video |
| Cinzel         | The wordmark and display headings               | OFL-1.1      | CDN only, never bundled                   |
| Go fonts       | Fallback when the video fonts are absent        | BSD-3-Clause | Compiled in from `x/image`                |
| Lucide         | Interface icons                                 | ISC          | Bundled with the frontend                 |

The story theatre's video export renders its frames in Go, so it cannot fetch a
typeface when it runs: the five TTF files under `pkg/scene/fonts/fonts/` are
embedded in the binary, and each family's `OFL-*.txt` is embedded beside it. The
OFL permits exactly this — the fonts "can be bundled, embedded, redistributed
and/or sold with any software", provided they are not sold by themselves and
their notice travels with them, which is what those files do. They are committed
so a build never needs the network; `go generate ./pkg/scene/fonts` refreshes
them. A checkout that has not run it falls back to the Go fonts, which are
compiled in from `golang.org/x/image` under the Go project's BSD licence.

Nothing about the interface is bundled. It links the Google Fonts CDN for Cinzel,
EB Garamond and JetBrains Mono and falls back to the system serif, sans and
monospace faces when they cannot be fetched, which is what an exported story
bundle uses when it runs from a `file://` URL with no network access. Cinzel is
used only that way, so it is never redistributed.

<!-- Generated from the Go module graph; refresh it when dependencies change. -->

## Go dependencies

LocalRPG links every one of these statically into the binary, so they are
distributed with it. Each group is the licence that module ships.

### MIT (59)

- `github.com/adrg/xdg`
- `github.com/alecthomas/assert/v2`
- `github.com/alecthomas/chroma/v2`
- `github.com/alecthomas/repr`
- `github.com/aymanbagabas/go-osc52/v2`
- `github.com/aymerick/douceur`
- `github.com/cenkalti/backoff/v5`
- `github.com/cespare/xxhash/v2`
- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/colorprofile`
- `github.com/charmbracelet/glamour`
- `github.com/charmbracelet/lipgloss`
- `github.com/charmbracelet/x/ansi`
- `github.com/charmbracelet/x/cellbuf`
- `github.com/charmbracelet/x/exp/golden`
- `github.com/charmbracelet/x/exp/slice`
- `github.com/charmbracelet/x/term`
- `github.com/chromedp/cdproto`
- `github.com/chromedp/chromedp`
- `github.com/chromedp/sysutil`
- `github.com/clipperhouse/displaywidth`
- `github.com/clipperhouse/uax29/v2`
- `github.com/darkliquid/roll`
- `github.com/dlclark/regexp2`
- `github.com/dlclark/regexp2/v2`
- `github.com/dop251/goja`
- `github.com/dustin/go-humanize`
- `github.com/erikgeiser/coninput`
- `github.com/felixge/httpsnoop`
- `github.com/go-ole/go-ole`
- `github.com/gobwas/httphead`
- `github.com/gobwas/pool`
- `github.com/gobwas/ws`
- `github.com/goccy/go-yaml`
- `github.com/gopxl/beep`
- `github.com/kr/pretty`
- `github.com/kr/text`
- `github.com/lucasb-eyer/go-colorful`
- `github.com/Masterminds/semver/v3`
- `github.com/matryer/is`
- `github.com/mattn/go-colorable`
- `github.com/mattn/go-isatty`
- `github.com/mattn/go-localereader (MIT stated in its README; the module ships no LICENSE file)`
- `github.com/mattn/go-runewidth`
- `github.com/muesli/ansi`
- `github.com/muesli/cancelreader`
- `github.com/muesli/reflow`
- `github.com/muesli/termenv`
- `github.com/ncruces/go-strftime`
- `github.com/orisano/pixelmatch`
- `github.com/pion/opus`
- `github.com/rivo/uniseg`
- `github.com/stretchr/testify`
- `github.com/wailsapp/wails/v3`
- `github.com/xo/terminfo`
- `github.com/yalue/onnxruntime_go`
- `github.com/yuin/goldmark`
- `github.com/yuin/goldmark-emoji`
- `go.uber.org/goleak`

### BSD-3-Clause (39)

- `github.com/go-json-experiment/json`
- `github.com/golang/protobuf`
- `github.com/google/go-cmp`
- `github.com/google/uuid`
- `github.com/googleapis/gax-go/v2`
- `github.com/gorilla/css`
- `github.com/grpc-ecosystem/grpc-gateway/v2`
- `github.com/hexops/gotextdiff`
- `github.com/ledongthuc/pdf`
- `github.com/microcosm-cc/bluemonday`
- `github.com/remyoudompheng/bigfft`
- `github.com/rogpeppe/go-internal`
- `github.com/srwiley/oksvg`
- `github.com/srwiley/rasterx`
- `golang.org/x/crypto`
- `golang.org/x/exp`
- `golang.org/x/image`
- `golang.org/x/mod`
- `golang.org/x/net`
- `golang.org/x/sync`
- `golang.org/x/sys`
- `golang.org/x/term`
- `golang.org/x/text`
- `golang.org/x/tools`
- `gonum.org/v1/gonum`
- `google.golang.org/protobuf`
- `modernc.org/cc/v4`
- `modernc.org/ccgo/v4`
- `modernc.org/fileutil`
- `modernc.org/gc/v2`
- `modernc.org/gc/v3`
- `modernc.org/goabi0`
- `modernc.org/libc`
- `modernc.org/mathutil`
- `modernc.org/memory`
- `modernc.org/opt`
- `modernc.org/sortutil`
- `modernc.org/strutil`
- `modernc.org/token`

### BSD-3-Clause / MIT (3)

- `github.com/aymanbagabas/go-udiff`
- `github.com/gen2brain/vpx`
- `modernc.org/sqlite`

### BSD-2-Clause (5)

- `github.com/go-sourcemap/sourcemap`
- `github.com/godbus/dbus/v5`
- `github.com/gorilla/websocket`
- `github.com/pkg/errors`
- `gopkg.in/check.v1`

### Apache-2.0 (39)

- `cloud.google.com/go`
- `cloud.google.com/go/auth`
- `cloud.google.com/go/compute/metadata`
- `github.com/at-wat/ebml-go`
- `github.com/ebitengine/oto/v3`
- `github.com/ebitengine/purego`
- `github.com/go-logr/logr`
- `github.com/go-logr/stdr`
- `github.com/google/pprof`
- `github.com/google/s2a-go`
- `github.com/googleapis/enterprise-certificate-proxy`
- `github.com/hajimehoshi/go-mp3`
- `github.com/k2-fsa/sherpa-onnx-go`
- `github.com/k2-fsa/sherpa-onnx-go-linux`
- `github.com/k2-fsa/sherpa-onnx-go-macos`
- `github.com/k2-fsa/sherpa-onnx-go-windows`
- `github.com/XSAM/otelsql`
- `go.opentelemetry.io/auto/sdk`
- `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`
- `go.opentelemetry.io/otel`
- `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc`
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc`
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace`
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`
- `go.opentelemetry.io/otel/log`
- `go.opentelemetry.io/otel/metric`
- `go.opentelemetry.io/otel/metric/x`
- `go.opentelemetry.io/otel/sdk`
- `go.opentelemetry.io/otel/sdk/log`
- `go.opentelemetry.io/otel/sdk/log/logtest`
- `go.opentelemetry.io/otel/sdk/metric`
- `go.opentelemetry.io/otel/trace`
- `go.opentelemetry.io/proto/otlp`
- `go.yaml.in/yaml/v3`
- `google.golang.org/genai`
- `google.golang.org/genproto/googleapis/api`
- `google.golang.org/genproto/googleapis/rpc`
- `google.golang.org/grpc`
- `gopkg.in/yaml.v3`

### ISC (1)

- `github.com/coder/websocket`

### MPL-2.0 (1)

- `github.com/hashicorp/golang-lru/v2`

## Licence texts

Every module above keeps its own licence beside its source, and the Go module
cache holds a copy (`go env GOMODCACHE`). The licences in play are:

- **MIT**, **ISC**, **BSD-2-Clause** and **BSD-3-Clause** — permissive; each
  asks that its copyright notice and permission notice travel with a
  distribution.
- **Apache-2.0** — permissive, with a patent grant and a notice requirement.
  Text at <https://www.apache.org/licenses/LICENSE-2.0>.
- **MPL-2.0** — file-level copyleft, satisfied by using the module unmodified.
  Text at <https://www.mozilla.org/MPL/2.0/>.

This file lists components rather than reproducing every licence text. Generate a
self-contained notice from the module cache before shipping a binary if one is
needed.
