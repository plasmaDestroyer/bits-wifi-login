# BITS Wifi Auto-Login Script

This script automatically logs you into the BITS Pilani Fortinet Captive Portal (`BITS-STUDENT` & `BITS-STAFF` Wi-Fi networks) in the background when your device connects to the network.

## ⚙️ Installation & Setup

There are automated install scripts for Linux, macOS, and Windows. They will prompt you for your BITS Wifi username and password to create a `creds.conf` file, and set up all the background triggers for your OS. To install just run one command based on your OS:

### 🐧 Linux
```bash
git clone https://github.com/plasmaDestroyer/bits-wifi-login.git && cd bits-wifi-login && ./linux/install.sh
```
*Requires NetworkManager. Sets up a NetworkManager dispatcher and a systemd background service.*

### 🍎 macOS
```bash
git clone https://github.com/plasmaDestroyer/bits-wifi-login.git && cd bits-wifi-login && ./mac/install.sh
```
*Sets up a background launchd agent that watches for Wi-Fi changes.*

### 🪟 Windows

Run in PowerShell (as Administrator).

**If you have Git installed** (check by running):
```powershell
git --version
```
If it returns something like `git version 2.x.x`, then run:
```powershell
git clone https://github.com/plasmaDestroyer/bits-wifi-login.git; cd bits-wifi-login; .\windows\install.ps1
```

**If you don't have Git:**
```powershell
Invoke-WebRequest https://github.com/plasmaDestroyer/bits-wifi-login/archive/refs/heads/main.zip -OutFile bits-wifi-login.zip; Expand-Archive bits-wifi-login.zip -DestinationPath .; cd bits-wifi-login-main; .\windows\install.ps1
```

*Registers a scheduled task that triggers instantly when you connect to a network.*

## 💤 Post-Installation

The installer creates a local `creds.conf` file for you. If you ever change your password or need to fix a typo, you can just edit that file directly.

That's it. From now on, whenever your device connects to `BITS-STUDENT` (or `BITS-STAFF` - they're essentially the same thing - in case you didn't know), you'll be logged in automatically without needing the Browser Captive Portal.

## 💡 Good to know

*   **Linux:** Fully tested and works like a charm (I use Arch btw 😉).
*   **macOS:** Should work well since it's essentially the same as linux.
*   **Windows:** Added recently, it should work great, though I haven't used it much as compared to linux.
*   **Issues?** If facing any issues, feel free to reach out to me or [open an issue on GitHub](https://github.com/plasmaDestroyer/bits-wifi-login/issues).


#### **Cheers 🍻**
