# ccmode

Switch between Claude Code auth / status-line profiles with one command, then
launch `claude`.

```
ccmode <profile> [claude args...]
```

Typical use: one profile points Claude Code at a company LiteLLM / proxy with an
API token, another uses your personal Max subscription via OAuth token. Instead
of hand-editing `~/.claude/settings.json` every time, run `ccmode work` or
`ccmode personal`.

> Working with an AI agent (Claude Code, Codex, etc.)? Point it at
> [AGENTS.md](AGENTS.md). It is a step-by-step runbook for installing ccmode and
> editing profiles safely.

## How it works

1. Reads `~/.ccmode/profiles.json`.
2. Reads `~/.claude/settings.json` (treated as empty if missing).
3. Saves a timestamped backup to `~/.ccmode/backups/settings.json.<unix-nanos>.bak`.
4. For each **managed key** (`env`, `statusLine`):
   - if the profile defines it, the value replaces whatever was in settings;
   - if the profile omits it, the key is **removed** from settings.
   Every other key in `settings.json` (permissions, hooks, model, etc.) is left
   byte-for-byte untouched.
5. Writes settings atomically (temp file + rename, mode 0600).
6. `exec`s `claude` from `PATH` with any extra arguments, replacing the ccmode
   process. Claude Code reads the new settings on startup.

There is no daemon, no state beyond the two JSON files, and no network access.
Single Go file, standard library only.

## Install

### Prebuilt binary (recommended)

Download from the [Releases](../../releases) page. Assets are named
`ccmode-<os>-<arch>`:

| Platform            | Asset                  |
|---------------------|------------------------|
| macOS Apple Silicon | `ccmode-darwin-arm64`  |
| macOS Intel         | `ccmode-darwin-amd64`  |
| Linux arm64         | `ccmode-linux-arm64`   |
| Linux x86_64        | `ccmode-linux-amd64`   |

```sh
# example: macOS Apple Silicon
curl -fsSL -o ccmode https://github.com/haliliceylan/ccmode/releases/latest/download/ccmode-darwin-arm64
chmod +x ccmode
mkdir -p ~/bin && mv ccmode ~/bin/ccmode
# macOS Gatekeeper: unsigned binary downloaded via browser may need
xattr -d com.apple.quarantine ~/bin/ccmode 2>/dev/null || true
```

Make sure `~/bin` is on your `PATH`. `SHA256SUMS` is attached to every release.

### go install

```sh
go install github.com/haliliceylan/ccmode@latest
```

### Build from source

Requires Go (version pinned in `go.mod`).

```sh
git clone https://github.com/haliliceylan/ccmode.git
cd ccmode
go build -o ~/bin/ccmode .
```

No Go on the machine? Use Docker:

```sh
docker run --rm -v "$PWD":/src -w /src -e GOOS=darwin -e GOARCH=arm64 \
  golang:1.27 go build -o ccmode .
```

## Configuration

Create `~/.ccmode/profiles.json`. It holds secrets, so restrict permissions:

```sh
mkdir -p ~/.ccmode && chmod 700 ~/.ccmode
touch ~/.ccmode/profiles.json && chmod 600 ~/.ccmode/profiles.json
```

Format: a JSON object keyed by profile name. Each profile may contain `env`
and/or `statusLine`. Values are copied verbatim into `settings.json`, so
anything Claude Code accepts under those keys is valid here.

```json
{
  "work": {
    "env": {
      "ANTHROPIC_BASE_URL": "https://llm.example.com",
      "ANTHROPIC_AUTH_TOKEN": "sk-...",
      "ANTHROPIC_CUSTOM_HEADERS": "x-litellm-api-key: Bearer sk-..."
    },
    "statusLine": {
      "type": "command",
      "command": "/Users/you/.claude/statusline-command.sh",
      "refreshInterval": 60
    }
  },
  "personal": {
    "env": {
      "CLAUDE_CODE_OAUTH_TOKEN": "sk-ant-oat01-..."
    }
  },
  "clean": {}
}
```

- `work` sets both a proxy and a custom status line.
- `personal` sets only an OAuth token. Because it omits `statusLine`, switching
  to it **removes** any status line from `settings.json`.
- `clean` removes both managed keys, handy for going back to plain `claude login`
  behaviour.

### Common `env` keys

| Key                          | Purpose                                              |
|------------------------------|------------------------------------------------------|
| `ANTHROPIC_BASE_URL`         | Proxy / gateway URL (LiteLLM, Bedrock proxy, etc.)   |
| `ANTHROPIC_AUTH_TOKEN`       | Bearer token sent to that URL                        |
| `ANTHROPIC_API_KEY`          | Direct Anthropic API key                             |
| `ANTHROPIC_CUSTOM_HEADERS`   | Extra headers, `"Name: value"` newline-separated     |
| `CLAUDE_CODE_OAUTH_TOKEN`    | Long-lived OAuth token from `claude setup-token`     |
| `ANTHROPIC_MODEL`            | Default model override                               |

Where to get an OAuth token for a subscription profile:

```sh
claude setup-token
```

## Usage

```sh
ccmode work                      # switch to "work", start claude
ccmode personal --continue       # extra args go straight to claude
ccmode work -p "summarise TODO"  # non-interactive prompt
ccmode                           # no args: prints available profiles, exit 1
ccmode --help
```

Because ccmode `exec`s claude, the shell sees claude's exit code, signals work
normally, and there is no wrapper process left behind.

## Backups and rollback

Every run writes a backup before touching settings:

```sh
ls -t ~/.ccmode/backups | head        # newest first
cp ~/.ccmode/backups/settings.json.<ts>.bak ~/.claude/settings.json
```

Backups are never pruned automatically. Delete old ones whenever you like.

## Troubleshooting

| Message | Cause / fix |
|---------|-------------|
| `no profiles file at ~/.ccmode/profiles.json (create it first)` | Create the file, see Configuration. |
| `parsing ~/.ccmode/profiles.json: ...` | Invalid JSON. Check with `python3 -m json.tool ~/.ccmode/profiles.json`. |
| `unknown profile "x"` | Name not in profiles.json. Run `ccmode` to list. |
| `claude not found in PATH` | Install Claude Code or add its install dir to `PATH`. |
| `parsing ~/.claude/settings.json: ...` | settings.json is corrupt. Restore from `~/.ccmode/backups/`. |
| Claude still uses old auth | Fully exit any running `claude`; settings are read at startup. |
| macOS: "cannot be opened because the developer cannot be verified" | `xattr -d com.apple.quarantine ~/bin/ccmode` |

## Security notes

- `profiles.json` contains live credentials. Keep it `0600`, never commit it,
  never paste it into chat or issues.
- `settings.json` will contain those same credentials after switching. It is
  written `0600`.
- Backups under `~/.ccmode/backups/` also contain credentials (`0600`).
- ccmode makes no network calls and has no dependencies outside the Go standard
  library. Read `main.go`; it is under 200 lines.

## Development

```sh
go test ./...        # unit test for the merge logic
go vet ./...
gofmt -l .           # must print nothing
```

CI (`.github/workflows/ci.yml`) runs those checks on every push and PR, then
cross-builds for macOS/Linux on arm64/amd64. Pushing a tag `v*` additionally
publishes a GitHub Release with all binaries and `SHA256SUMS`.

Release:

```sh
git tag v1.0.0 && git push origin v1.0.0
```

## License

MIT, see [LICENSE](LICENSE).
