---
id: 25-cli-reference
title: Command-Line Reference
category: Command Line & Terminal
order: 25
description: Every command, its flags, what it prints, and the exit code it leaves behind.
---

# Command-Line Reference

The binary provides one command per task. `gui` opens the desktop app, and `play`
opens the terminal client; the other commands do one job and exit.

```text
Usage: localrpg <command> [arguments]
```

Run `localrpg help`, or run the binary with no arguments, for the command index.
Output goes to standard output on success and to standard error on failure. A
command that succeeds exits `0`; a command that fails exits `1`. A shell script can
chain commands and trust the status.

## Commands at a glance

| Command | What it does |
| --- | --- |
| `roll <notation>` | Evaluate a dice expression. |
| `prompt [flags] <text>` | Send one prompt to a model and print the reply. |
| `play <game-id>` | Open the terminal client for a campaign. |
| `tts [flags] <text>` | Synthesize one utterance to an audio file. |
| `tts batch [flags] <game-id>` | Render a campaign's missing speech through a batch API. |
| `image <prompt>` | Print the image request for a prompt. |
| `gui [flags]` | Launch the desktop app. |
| `export web, video, or inspect [flags]` | Export a story replay, or inspect an exported bundle. |
| `content <cmd>` | Export, import, sign, or verify a content package. |
| `publisher <cmd>` | Manage the publishers whose signatures you trust. |
| `registry <cmd>` | Search, install, and update content from registries. |
| `debug <cmd>` | Run scenario tests, test a system, or start the debug server. |
| `config <cmd>` | Apply the offline preset, and check that nothing needs a network. |
| `styles <cmd>` | List or validate procedural style packs. |
| `version` | Print the version. |
| `help` | Print the command index. |

## `roll`

Evaluate dice notation without opening a campaign. This is the quickest way to
check that a system's notation parses before you commit it.

```bash
localrpg roll 1d20+5     # a d20 with a flat bonus
localrpg roll 4d6kh3     # keep the highest three of four
localrpg roll 4dF        # Fudge dice
```

The output lists the notation as parsed, the total, the number of successes when
the notation counts them, and how many dice were rolled.

| Argument | Meaning |
| --- | --- |
| `<notation>` | A dice expression. Required; a missing notation prints the usage line. |

## `prompt`

Send one prompt to a model and print the reply. It exists to test a provider
without a campaign in the way.

```bash
localrpg prompt "Summarise a tavern in one line"
localrpg prompt --cmd claude "Describe the weather"
localrpg prompt --endpoint http://localhost:11434 --model llama3.2 "Greet the party"
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--cmd <command>` | empty | Pipe the prompt through a command-line harness. |
| `--endpoint <url>` | empty | Send the prompt to an HTTP endpoint that speaks the OpenAI chat shape. |
| `--model <name>` | `llama3` | The model name sent to the HTTP endpoint. |

With neither `--cmd` nor `--endpoint`, the prompt goes to the local `echo` harness,
and the command works on a machine with nothing configured. The reply is printed
as it arrives, with no reformatting.

## `play`

Open the terminal client for one campaign. The session reloads the campaign from
disk, rebuilds its entity index, and plays turns through the same engine the
desktop app uses. See [The Terminal Client](26-terminal-client.md) for the keys and
the slash commands.

```bash
localrpg play my-campaign
```

| Argument | Meaning |
| --- | --- |
| `<game-id>` | The campaign's directory name under `games/`. Required. |

`--trace [level]` turns on the JSONL trace for the run; see
[Trace & Telemetry](29-trace-and-telemetry.md).

## `tts`

Synthesize one utterance and print the path of the audio file it wrote.

```bash
localrpg tts "The gate groans open."
localrpg tts --voice af_heart --pitch 0.9 --rate 1.1 "Steady now."
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--voice <id>` | the configured default voice | Override the voice for this utterance. |
| `--pitch <n>` | the configured pitch | Override the pitch. |
| `--rate <n>` | the configured speech rate | Override the speech rate. |

The voice and the prosody are part of the cache key, so two characters that share a
voice still get their own clip.

### `tts batch`

Render the speech a campaign is missing through a provider's batch API. A batch job
costs less than the interactive path and does not spend interactive rate-limit
quota.

```bash
localrpg tts batch my-campaign              # submit, and print the job id
localrpg tts batch --wait my-campaign       # submit and finish now
localrpg tts batch --status my-campaign     # what is in flight, and what failed
localrpg tts batch --resume <job-id> my-campaign
localrpg tts batch --cancel <job-id> my-campaign
```

| Flag | Meaning |
| --- | --- |
| `--wait` | Run the job to completion instead of returning after submission. |
| `--status` | Print each job's id, provider, status, and progress, and exit. |
| `--resume <job-id>` | Finish a job that was submitted earlier. |
| `--cancel <job-id>` | Cancel a job. |
| `--force` | Re-render every clip, overwriting the cached ones. |

A submitted job prints its id and the number of requests it contains. The clip
cache is the record of what is done, so a job interrupted by a restart can be
resumed rather than started again. The provider must offer a batch API; a provider
without one is reported, and the command exits `1`.

## `image`

`localrpg image <prompt>` prints the image request it would make for a prompt, and
exits. Use it to check the prompt-building path. Image generation itself runs
inside a campaign, where the workers call the configured image provider; see
[Story Theatre, Audio & Portraits](28-story-theatre.md).

## `gui`

Launch the desktop application. This is the default way to use LocalRPG.

```bash
localrpg gui                                    # native window
localrpg gui --headless                         # Unix socket, no window
localrpg gui --port 8080                        # 127.0.0.1:8080 for a browser
localrpg gui --dir ./my-project                 # treat this directory as the project root
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--port <n>` | `0` | Bind `127.0.0.1:<n>` and serve the app to a browser. |
| `--socket <path>` | the user runtime directory | Serve over a Unix domain socket at this path. |
| `--dir <path>` | `.` | Treat this directory as the project root, which also resolves relative `paths.*` against it. |
| `--headless` | `false` | Serve over a Unix domain socket with no window. |

The native window is the default, and it does not open a network socket. When
`$DISPLAY` and `$WAYLAND_DISPLAY` are both unset, the app falls back to the Unix
socket on its own.

## `export`

Compile a campaign into a replay and write it out, or inspect a bundle that was
written earlier.

```bash
localrpg export web my-campaign
localrpg export web my-campaign --out replay.html
localrpg export video my-campaign --fps 30 --size 1920x1080 --progress
localrpg export inspect replay.html
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--out <path>` | beside the campaign | Where to write the export. |
| `--dir <path>` | `.` | The campaign root to read. |
| `--no-art` | `false` | Skip scene imagery. |
| `--no-audio` | `false` | Skip speech clips and use only the cached ones. |
| `--still` | `false` | One frame per beat instead of animation. |
| `--fps <n>` | the scene default | Video frame rate. |
| `--size <WxH>` | `1920x1080` | Video size. |
| `--quality <n>` | `80` | Video quality, 0 to 100. |
| `--effort <n>` | the export default | Encoder effort, 0 to 6, where a higher number is slower and cleaner. |
| `--intro <duration>` | `1500ms` | Time the opening picture is shown before the story starts. |
| `--outro <duration>` | `1500ms` | Time the closing picture is shown after the story ends. |
| `--gap <duration>` | `300ms` | Extra time added to every segment beyond the script's own pacing. |
| `--progress` | `false` | Draw a live progress bar while rendering. |

`web` writes one self-contained HTML file: the theatre's own player, the story, and
every asset. It refuses to run without a built player, so build the frontend first.
`video` writes a WebM file. `inspect` takes a bundle path instead of a campaign and
prints a listing of every beat, including which clips are complete and which can be
decoded.

A beat with no speech clip is reported on standard error, together with the
directory the clips were searched in; pass `--dir` when a campaign is kept
elsewhere.

## `content`

Move content in and out of the installation as a signed package.

```bash
localrpg content export world my-world --out my-world.lrpgworld
localrpg content export system my-system --out my-system.lrpgsystem
localrpg content import my-world.lrpgworld --on-conflict rename --yes
localrpg content sign my-world.lrpgworld --key private.key --publisher me
localrpg content verify my-world.lrpgworld
```

| Command | Arguments |
| --- | --- |
| `content export` | `<world-or-system> <id> [--out <file>]` |
| `content import` | `<file> [--on-conflict refuse, rename, or overwrite] [--yes]` |
| `content sign` | `<file-or-dir> --key <private-key-file> [--publisher <name>]` |
| `content verify` | `<file-or-dir>` |

With no `--out`, `content export` writes the package to standard output, and a pipe
can send it anywhere. `content import` refuses a collision by default; `rename`
keeps both copies, and `overwrite` replaces the installed one. `--yes` skips the
confirmation prompt, which is what a script needs.

## `publisher`

Manage the publishers whose signatures `content verify` trusts.

```bash
localrpg publisher add public.key --name me
localrpg publisher add <fingerprint> --name me
localrpg publisher list
localrpg publisher remove <fingerprint>
```

A publisher entry is a display name and a public key, or a fingerprint that
references a key you already have.

## `registry`

Point LocalRPG at package registries and install from them.

```bash
localrpg registry add https://example.invalid/index.json
localrpg registry list
localrpg registry search tavern
localrpg registry install my-world@1.2.0 --on-conflict refuse
localrpg registry update
```

| Command | Arguments |
| --- | --- |
| `registry add` | `<url>` |
| `registry list` | none |
| `registry remove` | `<url>` |
| `registry search` | `[query]` |
| `registry install` | `<id>[@version] [--on-conflict refuse, rename, or overwrite] [--yes]` |
| `registry update` | none |

`registry install` accepts the same conflict modes as `content import`. `registry
update` reports the packages with a newer version available, and does not install
them.

## `debug`

Run the browser-driven scenario tests, test one system, or start the debug server
with its live dashboard. `docs/debugging.md` in the repository covers the scenarios
and the dashboard.

```bash
localrpg debug server --port 8080 --debugger-port 8089
localrpg debug test-run --scenario scenarios/smoke.yaml
localrpg debug test-system my-system
localrpg debug test-system --reference
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--scenario <path>` | none | The YAML scenario to run. |
| `--headless` | `true` | Run the browser without a visible window. |
| `--port <n>` | `8080` | The app's port. |
| `--debugger-port <n>` | `8089` | The dashboard's port. |
| `--report-dir <path>` | `test-results` | Where reports are written. |
| `--reference` | `false` | Test one of the built-in reference systems rather than one on disk. |
| `--system <id>` | none | The system to test; a positional id also works. |

The scenario tests use a real browser, and a machine with no usable browser skips
them rather than failing them.

## `config`

Apply the fully offline preset, or check that the configuration in force sends
nothing to the network.

```bash
localrpg config offline-preset
localrpg config offline-preset --tts sherpa-onnx
localrpg config check-offline
```

`config offline-preset` rewrites the provider configuration to the local engines
and saves it. `config check-offline` reports every configured provider that would
make a network request, and exits `1` when it finds one. See
[Local-First and Offline Play](21-local-first.md).

## `styles`

Work with the procedural style packs in the configuration directory.

```bash
localrpg styles list
localrpg styles validate my-pack.yaml
```

`styles validate` reads one pack and reports what it found. A pack with an unknown
structure or a malformed colour is reported and ignored in full, rather than half
applied. See [Style Packs](24-style-packs.md).

## `version`

```bash
localrpg version
```

Prints the version the binary was built with, which is the release tag for a
downloaded build.
