# AGENTS.md — runbook for AI agents

This file is for an AI coding agent (Claude Code, Codex, Cursor, etc.) that has
been asked to **install ccmode, create or modify profiles, switch profiles, or
fix a broken setup** on a user's machine. Follow it top to bottom. Every step
has a verification command; do not skip them.

If instead you are asked to **change ccmode's source code**, jump to
[Working on the source](#working-on-the-source).

---

## 0. Facts you need before doing anything

| Item | Value |
|------|-------|
| Binary name | `ccmode` |
| Invocation | `ccmode <profile> [claude args...]` |
| Profiles file | `~/.ccmode/profiles.json` (JSON object, keys = profile names) |
| Target file it edits | `~/.claude/settings.json` |
| Keys it manages | exactly two: `env`, `statusLine` |
| Keys it never touches | everything else in settings.json |
| Backups | `~/.ccmode/backups/settings.json.<unix-nanos>.bak`, one per run |
| After switching | it `exec`s `claude` from `PATH` (process is replaced) |
| Network access | none |
| Dependencies | none (Go stdlib only) |
| Source | single file `main.go`, ~190 lines. Read it if in doubt. |
| Repo | https://github.com/haliliceylan/ccmode |

Semantics that matter when editing profiles:

- A profile **with** `env` → that object replaces `env` in settings.json wholesale
  (not merged key-by-key).
- A profile **without** `env` → `env` is **deleted** from settings.json.
- Same rule for `statusLine`.
- An empty profile `{}` therefore removes both keys.

**Secrets:** `profiles.json`, `settings.json` and every backup contain live API
/ OAuth tokens. Never print their full contents into a transcript, log, issue,
or commit. When you must show them, redact values (`"sk-...REDACTED"`). Use
`jq 'del(..|.env?)'` or similar to inspect structure without values.

---

## 1. Detect the environment

```sh
uname -sm                     # Darwin arm64 | Darwin x86_64 | Linux aarch64 | Linux x86_64
command -v claude && claude --version
command -v ccmode && ccmode --help 2>&1 | head
command -v go && go version   # optional, only for building from source
test -f ~/.ccmode/profiles.json && echo "profiles exist" || echo "no profiles yet"
test -f ~/.claude/settings.json && echo "settings exist" || echo "no settings yet"
echo "$PATH" | tr ':' '\n' | grep -x "$HOME/bin" && echo "~/bin on PATH" || echo "~/bin NOT on PATH"
```

Map `uname -sm` to a release asset:

| `uname -sm`      | asset                  |
|------------------|------------------------|
| `Darwin arm64`   | `ccmode-darwin-arm64`  |
| `Darwin x86_64`  | `ccmode-darwin-amd64`  |
| `Linux aarch64`  | `ccmode-linux-arm64`   |
| `Linux x86_64`   | `ccmode-linux-amd64`   |

Stop and tell the user if `claude` is not installed. ccmode is useless without it.

---

## 2. Install the binary

Pick **one** method, in this order of preference.

### 2a. Prebuilt release (no toolchain needed)

```sh
ASSET=ccmode-darwin-arm64   # from the table above
mkdir -p ~/bin
curl -fsSL -o ~/bin/ccmode \
  "https://github.com/haliliceylan/ccmode/releases/latest/download/$ASSET"
chmod +x ~/bin/ccmode
# macOS only: clear Gatekeeper quarantine flag if present
[ "$(uname -s)" = Darwin ] && xattr -d com.apple.quarantine ~/bin/ccmode 2>/dev/null || true
```

Optional integrity check:

```sh
curl -fsSL https://github.com/haliliceylan/ccmode/releases/latest/download/SHA256SUMS \
  | grep "$ASSET" | sed "s#$ASSET#$HOME/bin/ccmode#" | shasum -a 256 -c -
```

If `~/bin` is not on `PATH`, append to the user's shell rc (`~/.zshrc` on macOS
default, `~/.bashrc` otherwise) and tell the user to open a new shell:

```sh
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc
```

### 2b. `go install`

```sh
go install github.com/haliliceylan/ccmode@latest   # lands in $(go env GOPATH)/bin
```

### 2c. Build from source

```sh
git clone https://github.com/haliliceylan/ccmode.git /tmp/ccmode-src
cd /tmp/ccmode-src && go build -o ~/bin/ccmode .
```

Without Go but with Docker (set GOOS/GOARCH to match the host):

```sh
docker run --rm -v /tmp/ccmode-src:/src -w /src \
  -e CGO_ENABLED=0 -e GOOS=darwin -e GOARCH=arm64 \
  golang:1.27 go build -o ccmode . && install -m755 /tmp/ccmode-src/ccmode ~/bin/ccmode
```

### Verify install

```sh
ccmode --help
```

Expected output (exit code 0 with `--help`):

```
usage: ccmode <profile> [claude args...]
no profiles defined            # or "available profiles:" + list
```

If it prints `no profiles file at ... (create it first)` that is also fine at
this stage; proceed to step 3.

---

## 3. Create `profiles.json`

### 3a. Create the directory with safe permissions

```sh
mkdir -p ~/.ccmode/backups
chmod 700 ~/.ccmode
```

### 3b. Ask the user what profiles they want

You need, per profile:

1. **name** (short, no spaces; it is what they will type: `ccmode <name>`)
2. **auth mechanism**, one of:
   - **proxy / gateway** (LiteLLM, corporate gateway): needs `ANTHROPIC_BASE_URL`
     and `ANTHROPIC_AUTH_TOKEN`; sometimes `ANTHROPIC_CUSTOM_HEADERS`.
   - **direct API key**: `ANTHROPIC_API_KEY`.
   - **subscription / OAuth**: `CLAUDE_CODE_OAUTH_TOKEN`. The user obtains it by
     running `claude setup-token` interactively (you cannot run this for them;
     it opens a browser).
   - **none**: an empty profile `{}` that resets Claude Code to default login.
3. optional **statusLine** object (only if they already use one; copy it from
   the current `~/.claude/settings.json` `.statusLine` if present).

Do not invent token values. Use a placeholder and tell the user exactly which
value to replace if they have not given you the real one.

### 3c. Write the file

Write with a heredoc or your file tool, then lock permissions:

```sh
cat > ~/.ccmode/profiles.json <<'EOF'
{
  "work": {
    "env": {
      "ANTHROPIC_BASE_URL": "https://llm.example.com",
      "ANTHROPIC_AUTH_TOKEN": "REPLACE_WITH_WORK_TOKEN"
    }
  },
  "personal": {
    "env": {
      "CLAUDE_CODE_OAUTH_TOKEN": "REPLACE_WITH_OUTPUT_OF_claude_setup-token"
    }
  },
  "default": {}
}
EOF
chmod 600 ~/.ccmode/profiles.json
```

If the user already has auth in `~/.claude/settings.json` and wants to keep it
as a profile, capture it instead of retyping:

```sh
jq -n --slurpfile s ~/.claude/settings.json \
  '{ current: ($s[0] | {env, statusLine} | with_entries(select(.value != null))) }' \
  > ~/.ccmode/profiles.json
chmod 600 ~/.ccmode/profiles.json
```

(then rename `current` to something meaningful with a follow-up edit).

### Verify

```sh
python3 -m json.tool ~/.ccmode/profiles.json > /dev/null && echo "valid JSON"
ls -l ~/.ccmode/profiles.json          # must show -rw------- (0600)
ccmode --help                           # lists profile names
```

---

## 4. Modify an existing `profiles.json`

Prefer `jq` for surgical edits so you never have to echo secrets. Always write
to a temp file and rename, and re-apply `chmod 600`.

Add or replace one env var in one profile:

```sh
jq --arg p work --arg k ANTHROPIC_MODEL --arg v claude-sonnet-5 \
   '.[$p].env[$k] = $v' ~/.ccmode/profiles.json > ~/.ccmode/profiles.json.tmp \
  && mv ~/.ccmode/profiles.json.tmp ~/.ccmode/profiles.json && chmod 600 ~/.ccmode/profiles.json
```

Remove an env var:

```sh
jq --arg p work --arg k ANTHROPIC_MODEL 'del(.[$p].env[$k])' ~/.ccmode/profiles.json \
  > ~/.ccmode/profiles.json.tmp && mv ~/.ccmode/profiles.json.tmp ~/.ccmode/profiles.json \
  && chmod 600 ~/.ccmode/profiles.json
```

Add a new profile:

```sh
jq --arg p staging --arg url https://staging-llm.example.com --arg tok REPLACE_ME \
   '.[$p] = {env: {ANTHROPIC_BASE_URL: $url, ANTHROPIC_AUTH_TOKEN: $tok}}' \
   ~/.ccmode/profiles.json > ~/.ccmode/profiles.json.tmp \
  && mv ~/.ccmode/profiles.json.tmp ~/.ccmode/profiles.json && chmod 600 ~/.ccmode/profiles.json
```

Rename a profile:

```sh
jq --arg from old --arg to new '.[$to] = .[$from] | del(.[$from])' ~/.ccmode/profiles.json \
  > ~/.ccmode/profiles.json.tmp && mv ~/.ccmode/profiles.json.tmp ~/.ccmode/profiles.json \
  && chmod 600 ~/.ccmode/profiles.json
```

Delete a profile:

```sh
jq --arg p old 'del(.[$p])' ~/.ccmode/profiles.json > ~/.ccmode/profiles.json.tmp \
  && mv ~/.ccmode/profiles.json.tmp ~/.ccmode/profiles.json && chmod 600 ~/.ccmode/profiles.json
```

Inspect structure **without leaking values**:

```sh
jq 'map_values({env: (.env // {} | keys), statusLine: (.statusLine != null)})' ~/.ccmode/profiles.json
```

If `jq` is unavailable, use `python3 -c` with `json.load`/`json.dump`
(`indent=2`) following the same temp-file + rename + chmod pattern. Do not use
`sed` on JSON.

### Verify after every edit

```sh
python3 -m json.tool ~/.ccmode/profiles.json > /dev/null && echo "valid JSON"
ccmode --help   # new/renamed profiles appear, deleted ones are gone
```

---

## 5. Switch profiles

```sh
ccmode work
```

This **replaces the current process with `claude`**. In an agent context that
means:

- Do not run it as a blocking foreground command expecting it to return; it
  starts an interactive Claude Code session.
- To verify a switch **without** launching an interactive session, pass a
  non-interactive claude flag, e.g. `ccmode work --version` or
  `ccmode work -p "reply with OK"`. ccmode still rewrites settings.json first.
- Any already-running `claude` keeps its old settings until restarted.

### Verify the switch took effect

```sh
jq '{env: (.env // {} | keys), statusLine: (.statusLine != null)}' ~/.claude/settings.json
ls -t ~/.ccmode/backups | head -1     # a fresh backup was created
```

The `env` key list must match the chosen profile's `env` keys exactly, and all
non-managed keys (e.g. `permissions`, `hooks`, `model`) must be unchanged
compared with the newest backup:

```sh
diff <(jq 'del(.env, .statusLine)' "$(ls -t ~/.ccmode/backups/* | head -1)") \
     <(jq 'del(.env, .statusLine)' ~/.claude/settings.json) && echo "unmanaged keys intact"
```

---

## 6. Rollback

Restore the most recent backup:

```sh
cp "$(ls -t ~/.ccmode/backups/* | head -1)" ~/.claude/settings.json
chmod 600 ~/.claude/settings.json
```

Restore a specific one: list with `ls -lt ~/.ccmode/backups`, pick by timestamp.

Backups are never auto-deleted. To prune, keep the newest N:

```sh
ls -t ~/.ccmode/backups/* | tail -n +21 | xargs rm -f   # keep 20
```

---

## 7. Uninstall

```sh
rm -f ~/bin/ccmode                    # or $(go env GOPATH)/bin/ccmode
rm -rf ~/.ccmode                      # removes profiles AND backups (secrets)
```

`~/.claude/settings.json` keeps whatever `env`/`statusLine` the last switch
wrote. Remove them manually if the user wants a clean state:

```sh
jq 'del(.env, .statusLine)' ~/.claude/settings.json > /tmp/s.json && mv /tmp/s.json ~/.claude/settings.json
```

---

## 8. Error → action table

| ccmode output | What to do |
|---------------|-----------|
| `no profiles file at ~/.ccmode/profiles.json (create it first)` | Step 3. |
| `parsing ~/.ccmode/profiles.json: ...` | Invalid JSON. `python3 -m json.tool` shows the line. Fix, never delete blindly. |
| `unknown profile "x"` + list | Typo or missing profile. Step 4 to add. |
| `claude not found in PATH` | Install Claude Code (`npm i -g @anthropic-ai/claude-code` or native installer) or fix PATH. |
| `parsing ~/.claude/settings.json: ...` | settings.json corrupt. Step 6 rollback, or if no backups, ask the user before replacing with `{}`. |
| `writing ~/.claude/settings.json.tmp: permission denied` | Check ownership/permissions of `~/.claude`. |
| Claude starts but ignores new auth | A previous `claude` is still running, or the profile's env keys are wrong for the auth type. Compare with the table in step 3b. |
| `zsh: command not found: ccmode` | `~/bin` not on PATH, or new shell not opened. Step 2. |
| macOS "developer cannot be verified" | `xattr -d com.apple.quarantine ~/bin/ccmode` |

---

## Working on the source

Layout:

```
main.go        everything: main → run → loadProfiles/loadSettings/backupSettings/applyProfile/writeSettings/printUsage
main_test.go   one test: applyProfile touches only managed keys
go.mod         module ccmode, no dependencies
.github/workflows/ci.yml   fmt+vet+test → 4-way cross build → release on v* tag
```

Invariants to preserve (tests or reviewers will reject otherwise):

1. Only keys listed in `managedKeys` may be written to or deleted from
   `settings.json`. Adding a managed key is a deliberate, documented change.
2. A backup is written **before** settings are modified, every run.
3. Settings are written atomically (temp + rename) with mode `0600`.
4. `claude` is launched via `syscall.Exec`, not as a child process.
5. No third-party dependencies. Standard library only.
6. `gofmt -l .` prints nothing.

Local checks:

```sh
gofmt -l . && go vet ./... && go test ./...
```

Without Go installed:

```sh
docker run --rm -v "$PWD":/src -w /src golang:1.27 sh -c 'gofmt -l . && go vet ./... && go test ./...'
```

Release: bump nothing (no version constant), tag and push. CI builds and
uploads the four binaries plus `SHA256SUMS`.

```sh
git tag v1.x.y && git push origin v1.x.y
```

Commit style: short imperative subject, body explains *why* when non-obvious.
