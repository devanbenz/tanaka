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

Homebrew (macOS, Linux):

    brew install devanbenz/tap/tanaka

Scoop (Windows):

    scoop bucket add devanbenz https://github.com/devanbenz/scoop-bucket
    scoop install tanaka

Prebuilt binaries for every platform are on the
[releases page](https://github.com/devanbenz/tanaka/releases).

Or build from source:

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

Export your progress as an Obsidian mind map — a folder of wikilinked notes
(sections, questions with hidden answers, key concepts) that Obsidian's
graph view renders as a connected web. Only questions you've answered with a
passing (or partial) verdict are included, so the map reflects what you've
actually learned; with no completed questions there is nothing to export:

    tanaka export <id> --format obsidian            # writes ./<slug>/
    tanaka export <id> --format obsidian -o ~/vault/tanaka

Or sync live while you study: with --obsidian-dir set, every question you
answer correctly adds its note (and its section's note) to the vault, so the
map builds itself as you go. Notes are never deleted, only added or updated:

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

