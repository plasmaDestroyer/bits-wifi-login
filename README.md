# BITS Wifi Auto-Login

Wi-Fi says connected, nothing loads, and the captive portal wants your username and password again. For the third time today.

This logs you into the BITS Pilani Fortinet portal (`BITS-STUDENT` or `BITS-STAFF`) in the background, usually before you notice you got kicked out.

It's one small Go binary and there's nothing else to install (on Linux it uses NetworkManager and systemd, which you almost certainly have). The installer downloads the right build for your OS and hooks it into the OS's own background triggers. After that you can forget about it.

## ⚙️ Install

### 🐧 Linux / 🍎 macOS
```bash
curl -fsSL https://plasmaDestroyer.github.io/bits-wifi-login/install.sh | bash
```

### 🪟 Windows
Open PowerShell and run this. If it says "Access is denied", reopen PowerShell as Administrator and run it again.
```powershell
irm https://plasmaDestroyer.github.io/bits-wifi-login/install.ps1 | iex
```

It asks for your BITS Wi-Fi username and password once and saves them to `creds.conf`. Changed your password or typed it wrong? Edit `creds.conf`.

### 📂 Where it lives

| Platform | Folder |
|---|---|
| 🐧 Linux / 🍎 macOS | `~/.local/share/bits-wifi-login/` |
| 🪟 Windows | `%LOCALAPPDATA%\bits-wifi-login\` |

The binary, `creds.conf` and (on Windows) `bits-wifi-login.log` all sit in that folder.

You don't have to remember the path. Install puts `bits-wifi-login` on your PATH, by adding the folder to your user PATH on Windows and by symlinking it into `~/.local/bin` on Linux and macOS.

On Windows, close the terminal you installed from and open a new one. A program can't change the environment of the shell that started it, so the old window will keep saying "not recognized" no matter how long you wait.

On Linux and macOS it works right away. The one exception is when the installer says `~/.local/bin` isn't on your PATH (the macOS default). It prints the line to add when that happens.

### 🩺 Is it working?

```bash
bits-wifi-login status
```

This prints where the files are, whether each background trigger is still registered, which network you're on, and whether you're logged in right now. On Windows it also shows what the last run did.

## 🧠 How it works

1. It requests `connectivitycheck.gstatic.com/generate_204`. A `204` back means you're online and there's nothing to do (unless your session is about to expire, more on that below).
2. If the Fortinet intercepts that request, it pulls the magic token out of the portal's reply.
3. It uses the token to start a session, posts your credentials, and follows the `keepalive` link. That link only shows up when the login worked.

Your OS starts it:

- **Linux:** NetworkManager on every Wi-Fi connect, resume from sleep, and a 10-minute timer as a fallback. If NetworkManager's own connectivity check works on your machine (the installer tests this), it also notices a dropped session within about 20 seconds.
- **macOS:** a launchd agent that runs on network changes, at login, and every 5 minutes.
- **Windows:** two scheduled tasks that run at logon, on network connect, on wake, and every 5 minutes.

Portal sessions expire on a fixed timer. It's about 12 hours, whatever the portal page says. After the tool has seen a session or two expire it knows the real length, and it starts watching just before the next expiry. Most of the time you're back online a few seconds after the drop, so you don't find out ten minutes later when YouTube stops.

## ⬆️ Updating

Run the install command again. It downloads the newest release over the old binary and re-registers the triggers. Your `creds.conf` stays as it is.

## 🗑️ Uninstalling

```bash
bits-wifi-login uninstall
```

This removes the background triggers and the PATH entry. The binary, `creds.conf` and the log stay, so delete the folder above when you're done. On Linux also delete `~/.local/state/bits-wifi-login/`, which is where the NetworkManager log goes. `creds.conf` is kept on purpose so a reinstall doesn't ask for your password again.

You can run `uninstall` twice. The second time it tells you there was nothing to remove.

> **⚠️ Don't move the install folder.** The background triggers (systemd units, launchd plist, scheduled tasks) point at the binary by its full path. If you moved it anyway, run `./bits-wifi-login install` (Windows: `.\bits-wifi-login.exe install`) from the new folder and it fixes itself.

## 🔧 Troubleshooting

First, run `bits-wifi-login install` again. It re-registers everything, so it's also the repair command for broken triggers, permission problems and installs that stopped halfway. It doesn't touch an existing `creds.conf`, so a wrong password gets fixed by editing that file. If the binary itself is gone, run the download command from the top again.

To see what it's doing, run it in the foreground. It prints every step:

```bash
bits-wifi-login
```

Logs:

- **Linux:** `journalctl -u bits-wifi-login.service --since today`, plus `~/.local/state/bits-wifi-login/dispatcher.log` for the NetworkManager side.
- **macOS:** background runs don't keep a log yet. Run `bits-wifi-login` in a terminal and watch.
- **Windows:** `bits-wifi-login.log` next to the binary. Every run adds a line, background runs too, so an empty log almost always means the triggers never ran (check `bits-wifi-login status`). Once it's over 512 KB it gets moved to `.log.1`, so it doesn't keep growing.

### 🕳️ VPNs (Cloudflare WARP, Tailscale etc.)

A VPN can send the connectivity check through its tunnel and past the portal, so the tool never sees the login page. You'll get `no magic token found`, or it decides you're online when you're not. Logging in from the browser still works, which makes it confusing to track down.

For Cloudflare WARP you need both of these commands. With only the first one it looks fixed until the moment your session expires:

```bash
warp-cli tunnel host add connectivitycheck.gstatic.com
warp-cli dns fallback add connectivitycheck.gstatic.com
```

For any other VPN, exclude `connectivitycheck.gstatic.com` from the tunnel, and make sure its DNS lookup goes outside the tunnel too.

### 🔒 Certificate errors

Your password goes to the portal over HTTPS with TLS verification always on. There's no flag to turn it off. The portal's certificate is publicly trusted, so if verification fails, something is wrong with the connection. Please report it and don't work around it.

## 💡 Good to know

*   **Linux:** Fully tested and works great (I use Arch btw 😉).
*   **macOS:** Should work fine, it's close enough to Linux.
*   **Windows:** Works, but I've used it a lot less than Linux.
*   **`BITS-STUDENT` vs `BITS-STAFF`:** same portal and same login, in case you didn't know. Both work.
*   **Issues?** Message me or [open an issue on GitHub](https://github.com/plasmaDestroyer/bits-wifi-login/issues). If the tool saved a `fortinet_error_*.html` file, attach it, but open it first. The password is removed from the login-response dumps, but the other dumps are saved exactly as the portal sent them.

## 🛠️ Building from source

You only need this if there's no prebuilt binary for your platform or you want to change something. You need Go 1.26.3 or newer:

```bash
git clone https://github.com/plasmaDestroyer/bits-wifi-login
cd bits-wifi-login
go build -o bits-wifi-login ./cmd/bits-wifi-login
./bits-wifi-login install
```

`creds.conf` is read from the folder the binary is in, so keep them together.


#### **Cheers 🍻**
