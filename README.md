# Tanaka

Turn technical content (papers, blog posts, articles) into a study-then-build
learning flow. This milestone covers ingestion: importing content into
structured sections.

## Requirements

- Go 1.26+
- The `claude` CLI, logged in (`claude login`)

## Install

    go build -o tanaka .

## Usage

Add content from a file, URL, or stdin:

    tanaka add paper.pdf
    tanaka add https://example.com/post
    cat notes.md | tanaka add -

List what you have imported:

    tanaka list

Show help (also shown when run with no command):

    tanaka help

## How it works

`tanaka add` reads the raw content and asks the `claude` CLI to clean it into
Markdown and split it into ordered sections, then stores the result in
`~/.tanaka/tanaka.db`. Later milestones add the study UI and the build phase.
