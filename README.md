# BITS Wifi Auto-Login Script

This script automatically logs you into the BITS Pilani Fortinet Captive Portal (`BITS-STUDENT` & `BITS-STAFF` Wi-Fi networks) in the background when your device connects to the network.

## ⚙️ Installation & Setup

There are automated install scripts for Linux, macOS, and Windows. To install just run one command based on your OS:

### 🐧 Linux
```bash
curl -fsSL https://raw.githubusercontent.com/plasmaDestroyer/bits-wifi-login/main/linux/remote-install.sh | bash
```
*Requires NetworkManager. Sets up a NetworkManager dispatcher and a systemd background service.*

### 🍎 macOS
```bash
curl -fsSL https://raw.githubusercontent.com/plasmaDestroyer/bits-wifi-login/main/mac/remote-install.sh | bash
```
*Sets up a background launchd agent that watches for Wi-Fi changes.*

### 🪟 Windows
Open PowerShell as Administrator and run:
```powershell
irm https://raw.githubusercontent.com/plasmaDestroyer/bits-wifi-login/main/windows/remote-install.ps1 | iex
```
*Registers a scheduled task that triggers on network connect.*

After installation, it will prompt you for your BITS Wifi username and password to create a `creds.conf` file, and set up all the background triggers for your OS. If you ever change your password or need to fix a typo, you can just edit that file directly.

## 💤 Post-Installation

That's it. From now on, whenever your device connects to `BITS-STUDENT` (or `BITS-STAFF` - they're essentially the same thing - in case you didn't know), you'll be logged in automatically without needing the Browser Captive Portal.

## 💡 Good to know

*   **Linux:** Fully tested and works like a charm (I use Arch btw 😉).
*   **macOS:** Should work well since it's essentially the same as linux.
*   **Windows:** Added recently, it should work great, though I haven't used it much as compared to linux.
*   **Issues?** If facing any issues, feel free to reach out to me or [open an issue on GitHub](https://github.com/plasmaDestroyer/bits-wifi-login/issues).


#### **Cheers 🍻**
