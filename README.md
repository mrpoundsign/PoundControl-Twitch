<p align="center">
  <img src="assets/logo.svg" alt="PoundControl-Twitch Logo" width="128" height="128" />
</p>

# PoundControl-Twitch

A sandboxed WebAssembly (`.wasm`) Twitch plugin for [PoundControl](https://github.com/mrpoundsign/PoundControl), providing full Elgato Stream Deck Twitch parity using **Twitch OAuth 2.0 Device Code Grant Flow** (no client secret or local callback server required).

## Actions Included (12)

- **Stream Title & Category** — Updates stream title, game/category (exact match with fuzzy category fallback), broadcast language, and tags.
- **Chat Message** — Sends a preset message or command to your channel (or a configured target channel).
- **Clear Chat** — Wipes chat messages.
- **Emote-Only Chat** — Toggle / Force On / Force Off with live status feedback.
- **Followers-Only Chat** — Toggle / Force On / Force Off with configurable minimum follow duration (`0m`–`3mo`) and live status feedback.
- **Sub-Only Chat** — Toggle / Force On / Force Off with live status feedback.
- **Slow Chat** — Toggle / Force On / Force Off with configurable cooldown (`3s`–`120s`) and live status feedback.
- **Shield Mode** — Toggle / Force On / Force Off Twitch Shield Mode with live status feedback.
- **Play Ad** — Starts a `30s`–`180s` commercial break.
- **Create Clip** — Creates a clip from the live broadcast, logs the clip URL, and optionally posts it to chat.
- **Stream Marker** — Drops a VOD highlight marker with optional description.
- **Viewer Count** — Displays live concurrent viewer count (or `Offline`) on a shared background polling timer; press to refresh immediately.

## Building

Requires **Go 1.27+**.

### Windows (PowerShell)
```powershell
.\build.ps1
# Or bake in a default Twitch Application Client ID:
.\build.ps1 -ClientId "your_twitch_client_id"
```

### Linux / macOS / WSL
```bash
./build.sh
# Or bake in a default Twitch Application Client ID:
TWITCH_CLIENT_ID="your_twitch_client_id" ./build.sh
```

Output is written to `build/bin/poundcontrol-twitch.wasm`.

## Setup in PoundControl

1. Copy `build/bin/poundcontrol-twitch.wasm` into PoundControl's WebAssembly plugins folder (or add `build/bin/` as an extra Wasm plugin path in PoundControl Settings).
2. Open **Settings → Twitch** (via the gear icon next to the **Twitch** category in the Action Catalog, or **Plugins → Configure**).
3. Click **Sign In with Twitch**.
4. Copy the generated **Activation Code**, click the **Twitch Activation Page** link (`twitch.tv/activate`), and authorize PoundControl.
5. Once authorized, your session updates automatically and OAuth tokens are stored securely in your OS keyring (automatically refreshed as needed). You can optionally override the **Twitch Client ID** under **Advanced**.
