---
id: 26-terminal-client
title: The Terminal Client
category: Command Line & Terminal
order: 26
description: The keys, action modes, and slash commands for playing a campaign in a terminal.
---

# The Terminal Client

`localrpg play <game-id>` opens a campaign in the terminal. The client plays turns
through the same engine the desktop app uses, and writes every turn to the same
`history.jsonl`, so a campaign can be moved between the two at any time.

The client runs in the terminal's alternate screen: it takes the window over, and
restores what was there when you quit. A terminal that reports its size and
supports colour is enough. Narration is rendered as Markdown at the terminal's
width.

## Starting a session

Opening a campaign does the same preparation as the desktop app, in this order:

- The configuration is loaded, and the campaign's directories are resolved.
- The entity index is rebuilt from the Markdown, so edits made outside the app are
  picked up.
- The turn index is replayed from `history.jsonl` when the two disagree.
- The player's identity and the starting location are resolved.
- When the campaign has an opening prompt and no turns yet, the opening scene is
  played and shown before the prompt, which is the scene the desktop app shows in
  its Prologue.

Background embedding work runs as it does in the app, so semantic search over the
campaign is available even though the terminal client has no search screen.

## The screen

The window has three bands. The title reads **LocalRPG Chronicles**. The
transcript follows, and it shows each turn as the mode, what you typed, and the
narrator's prose. The last band is the status line and the input line. The
status line shows the current location in brackets, followed by the last thing that
happened, such as **Ready.**, **GM is thinking…**, or **Turn 4 completed.**, or the
text of an error. The input line shows the active mode in brackets, as in
`[Do] >`.

## Keys

| Key | What it does |
| --- | --- |
| `Tab` | Cycle the action mode through Do, Say, Story, Roll, and GM. |
| `Enter` | Submit what you have typed. |
| `Esc` | Clear the input line. |
| `Ctrl+C` | Quit. |

Enter does nothing while a turn is in flight. A second submission cannot interleave
with the first. The narrator's prose appears when the turn is complete; the client
does not stream it.

## Action modes

The mode selects how the engine reads what you typed, and the turn records it.

| Mode | Read as |
| --- | --- |
| **Do** | Something the character does. |
| **Say** | Something the character says. |
| **Story** | A narration or a scene-setting statement. |
| **Roll** | A request for a check, when the campaign's mechanics are engaged. |
| **GM** | A director's note, which corrects the last turn rather than playing a new one. |

The desktop app's console tabs use the same four action modes, and a turn plays
the same way in either client.

## Slash commands

Input that begins with a slash is read as a command rather than an action. Each
command is answered with its own entry in the transcript.

| Command | What it does |
| --- | --- |
| `/undo` | Rewind the campaign by one turn, deleting that turn's indexed record and the history it added to entities. |
| `/recap` | Ask where the campaign stands. The story so far is regenerated first when it is stale; the answer describes the present. |
| `/go <location>` | Move the player to a named location, recorded as its own system turn. |
| `/gm <directive>` | Correct the narration. The GM mode does the same thing. |

`/undo` refuses when there is nothing to undo, and `/go` refuses a location it
cannot find. Either failure is printed in the status line, and the session
continues.

## Diagnosing a turn

An error is printed in the status line, and the transcript keeps the turns that
already completed. For anything deeper, run with the trace on:

```bash
localrpg play my-campaign --trace summary
```

The trace records prompt sizes, timings, and provider decisions; `--trace full` adds
prompts, generated text, and raw provider lines. See
[Trace & Telemetry](29-trace-and-telemetry.md) for the levels and for the location
of the file.

## Differences from the desktop app

The terminal client is the same engine with fewer screens. The desktop app adds
what a terminal cannot show:

| Feature | Terminal | Desktop app |
| --- | --- | --- |
| Narration | Yes | Yes |
| Scene artwork | No | Yes |
| Spoken narration and per-line audio | No | Yes |
| Dice cards for a pending check | No, a Roll is typed instead | Yes |
| Character sheet, graph, and Codex drawers | No | Yes |
| Settings and studios | No, edit the configuration file instead | Yes |

Use the desktop app to author worlds, systems, and campaigns. The terminal client is
for playing them.
