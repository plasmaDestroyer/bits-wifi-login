# BITS Wifi Auto-Login

Automatically logs you into the BITS Pilani Fortinet Captive Portal (`BITS-STUDENT` & `BITS-STAFF` Wi-Fi networks) in the background when your device connects to the network.

It's a single small Go binary with no runtime dependencies — the installer downloads the prebuilt one for your platform and wires up your OS's native background triggers around it.

## ⚙️ Installation & Setup

### 🐧 Linux / 🍎 macOS
```bash
curl -fsSL https://plasmaDestroyer.github.io/bits-wifi-login/install.sh | bash
```
*Linux requires NetworkManager, and sets up a dispatcher hook plus a systemd service and timer. macOS gets a launchd agent that triggers on DNS/network changes (fires on most Wi-Fi connects but is not a precise trigger — the 5-minute periodic fallback is the reliable safety net).*

### 🪟 Windows
Open PowerShell as Administrator and run:
```powershell
irm https://plasmaDestroyer.github.io/bits-wifi-login/install.ps1 | iex
```
*Registers scheduled tasks that trigger on network connect, on resume, on login, and every 5 minutes.*

After installation, it will prompt you for your BITS Wifi username and password to create a `creds.conf` file, and set up all the background triggers for your OS. If you ever change your password or need to fix a typo, you can just edit that file directly.

To remove the background triggers:

```bash
bits-wifi-login uninstall
```

`creds.conf` is left behind on purpose so a reinstall doesn't re-prompt. Delete it yourself if you're done.

> **Note:** Do not move the install directory after setup. The installer bakes absolute paths into the background trigger configs (systemd unit, launchd plist, or scheduled task), so moving the directory will break the auto-login triggers. If you ever need to relocate it, just re-run the install script from the new location and it will repair everything.

## 💤 Post-Installation

That's it. From now on, whenever your device connects to `BITS-STUDENT` (or `BITS-STAFF` - they're essentially the same thing - in case you didn't know), you'll be logged in automatically without needing the Browser Captive Portal.

## 🔧 Troubleshooting

If something breaks (missing files, broken hooks, permission errors, or partial installs), just run **`bits-wifi-login install`** again. It re-registers every component, so it doubles as a repair command.

To see what's happening, run it in the foreground — it prints each step:

```bash
bits-wifi-login
```

### Certificate errors

Your credentials are sent to the portal over HTTPS with **TLS verification always on** — there is no
flag to turn it off, by design. The portal's certificate is publicly trusted, so a verification
failure means something is genuinely wrong with the connection, not with this tool. Report it rather
than working around it.

## 💡 Good to know

*   **Linux:** Fully tested and works fine (I use Arch btw 😉).
*   **macOS:** Should work well since it's essentially the same as linux.
*   **Windows:** Added recently, it should work great, though I haven't used it much as compared to linux.
*   **Issues?** If facing any issues, feel free to reach out to me or [open an issue on GitHub](https://github.com/plasmaDestroyer/bits-wifi-login/issues).

## 🛠️ Building from source

Only needed if there's no prebuilt binary for your platform, or you're hacking on it:

```bash
git clone https://github.com/plasmaDestroyer/bits-wifi-login
cd bits-wifi-login
go build -o bits-wifi-login ./cmd/bits-wifi-login
./bits-wifi-login install
```

`creds.conf` is read from beside the binary, so keep them together.


#### **Cheers 🍻**
