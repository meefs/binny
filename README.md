# BINNY

BINNY is OpenBin's terminal agent for reverse engineering. It opens one of
your OpenBin or OpenAPK projects, reads the decompiled code through the same
backend the web app uses, and runs a triage, investigate, draft loop with
your own LLM key. The draft lands on the project report for you to review.

## Install

macOS / Linux:

    curl -fsSL https://openbin.ai/install-binny.sh | sh

Windows (PowerShell):

    irm https://openbin.ai/install-binny.ps1 | iex

Then:

    openbin login   # once; BINNY reuses the openbin CLI's sign-in
    binny           # pick a project, set a key, go

Docker is needed only for `/decompile` (local Ghidra) and the sandbox tools.

## Providers

BYOK. Anthropic, OpenAI, DeepSeek, Qwen, Kimi, Gemini (OpenAI-compatible)
and Bedrock are preconfigured; `/key` stores a key, `/model` switches.
`/endpoint <url>` points a provider at another host, for example a gateway
that fronts the Claude Messages API on your own domain. Config lives in
`~/.config/binny/config.json`.

## Commands

Type `/` in the prompt for the palette, or `/help` for the full list.

## Build from source

    go build -o binny .
    go test ./...

Releases are cut by tagging `v*` (see `.github/workflows/release.yml`) or
with `scripts/release-local.sh <tag>`. Binaries are published to the public
`openbin-ai/binny-releases` repo; this repo stays private.

## License

BINNY is licensed under the [GNU Affero General Public License v3.0 or later](LICENSE).
Contributors must sign the [Contributor License Agreement](CLA.md); the CLA
Assistant bot will prompt you on your first pull request.
