# Tanaka

Turn technical content (papers, blog posts, articles) into a study-then-build
learning flow: import a source, study it with quizzes, then implement it
yourself in a language of your choice.

> "Well, Mr. Average, do you really have time to be hanging your head?"
> — Tanaka Ryūnosuke, *Haikyū!!*

Named after Tanaka: the tool is meant to push someone who thinks they're average
toward great heights. It does not coddle — it makes you struggle productively.

## Requirements

- Go 1.26+
- The `claude` CLI, logged in (`claude login`)
- For the build phase, the toolchain of whatever language you pick
  (`cargo`, `go`, `pytest`, `make`, ...)

## Install

    go build -o tanaka .

## Usage

Import a source from a file, URL, or stdin:

    tanaka add paper.pdf
    tanaka add https://example.com/post
    cat notes.md | tanaka add -

List or remove sources (ids come from `tanaka list`):

    tanaka list
    tanaka remove <id>

Start the local web UI and open http://127.0.0.1:7777:

    tanaka serve

From the web UI you can:

- study a source — read it section by section and answer quizzes (multiple
  choice and free response) to unlock the next section
- build it — pick a language and difficulty, get a scaffolded workspace with
  acceptance tests, implement each step in your editor, and run the tests in
  the browser to advance

You can also drive those phases from the CLI:

    tanaka prepare <id>              # generate the study package up front
    tanaka build <id> --lang go      # scaffold a build workspace
    tanaka build <id> --lang rust --difficulty blank-page

Difficulty is one of `guided`, `spec+tests` (default), or `blank-page`.
Supported build languages: `rust`, `go`, `cpp`, `c`, `python`.

Run with no command (or `tanaka help`) to see the command list.

## How it works

Everything is stored in `~/.tanaka/` (a SQLite database plus build workspaces
under `~/.tanaka/builds/`). Tanaka shells out to the `claude` CLI for the
"smart" steps — cleaning content into sections, writing quizzes, grading free
answers, generating a build plan, and giving hints — so it runs on your Claude
subscription with no API key. Multiple-choice grading and running your build
tests happen locally, no model involved.
