# owui

A terminal client for an [Open WebUI](https://github.com/open-webui/open-webui) server. Chats started here are stored on that server. The sidebar lists those chats, and the web UI shows them too.

## Build

```bash
go build -ldflags "-X main.version=v1.0.0" -o owui ./cmd/owui
```

The splash screen shows that tag. A build without `-X` shows `dev`. Installing a tagged release with `go install github.com/papodaca/owui/cmd/owui@v1.0.0` shows the module version Go records for that install.

## Run

```bash
./owui
./owui -url http://localhost:8080
```

Settings live in `$XDG_CONFIG_HOME/owui/config.toml`, which is `~/.config/owui/config.toml` when `XDG_CONFIG_HOME` is unset. The file is mode 0600 because it holds a key.

```toml
url = "http://localhost:8080"
api_key = "sk-..."
```

`OWUI_URL` and `OWUI_API_KEY` override the file. `OWUI_CONFIG` picks a different path. `-url` overrides the file and `OWUI_URL`. `-config` picks a different file.

With no key saved, owui asks for the server URL and either an API key or an email and password. A password login stores the session token in the same file.

The list of chats this client created is `$XDG_STATE_HOME/owui/chats.json`, or `~/.local/state/owui/chats.json`.

Type `/help` for keys. Ctrl+C twice quits.
