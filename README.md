# Tanaka

Turn technical content (papers, blog posts, articles) into a study-then-build
learning flow: import a source, study it with quizzes, then implement it
yourself in a language of your choice.

> "Well, Mr. Average, do you really have time to be hanging your head?"
> — Tanaka Ryūnosuke, *Haikyū!!*

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

Share a source and its quizzes as a single `.tanaka` file, and import one
someone shared with you (imported files always create a fresh source):

    tanaka export <id>               # writes <slug>.tanaka
    tanaka export <id> -o sheet.tanaka
    tanaka import sheet.tanaka

Export a source as an Obsidian mind map — a folder of wikilinked notes
(sections, questions with hidden answers, key concepts) that Obsidian's
graph view renders as a connected web:

    tanaka export <id> --format obsidian            # writes ./<slug>/
    tanaka export <id> --format obsidian -o ~/vault/tanaka

Or sync live while you study: with --obsidian-dir set, every section you
pass or skip regenerates that source's notes (including your answers):

    tanaka serve --obsidian-dir ~/vault/tanaka

Start the local web UI and open http://127.0.0.1:7777:

    tanaka serve

From the web UI you can:

- study a source: read it section by section and answer quizzes (multiple
  choice and free response) to unlock the next section
- build it: pick a language and difficulty, get a scaffolded workspace with
  acceptance tests, implement each step in your editor, and run the tests in
  the browser to advance
- export a source to a `.tanaka` file, or import one from another user

You can also drive those phases from the CLI:

    tanaka prepare <id>              # generate the study package up front
    tanaka build <id> --lang go      # scaffold a build workspace
    tanaka build <id> --lang rust --difficulty blank-page

Difficulty is one of `guided`, `spec+tests` (default), or `blank-page`.
Supported build languages: `rust`, `go`, `cpp`, `c`, `python`.

