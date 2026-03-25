# BITS Wifi Auto-Login Script

This script automatically logs you into the BITS Pilani Fortinet Captive Portal (`BITS-STUDENT` and `BITS-STAFF` Wi-Fi networks) completely in the background the moment your device connects to the network.

## ⚙️ Installation & Setup

Follow these exact steps to set up the auto-login system.

### 1. Clone the Repository
Open your terminal and clone the script to your machine:
```bash
git clone https://github.com/plasmaDestroyer/bits-wifi-login.git
cd bits-wifi-login
```

### 2. Set Up Your Credentials (`creds.conf`)
The script needs your BITS username and password to log you in. These must be stored in a file named `creds.conf` in the exact same directory as the `fortinet-login.sh` script.

Create a file named `creds.conf` inside the `bits-wifi-login` directory:
```bash
touch creds.conf
```
Open `creds.conf` in your favorite text editor and exactly add your BITS username and password like this:
```bash
USERNAME="f20XXXXXXXXX"
PASSWORD="your_password_here"
```
*(Make sure not to commit this file to GitHub! The repo already contains a `.gitignore` ignoring `.conf` files.)*

**🔒 Security Note:** Because this file contains your password, you should ensure no other users on your computer can read it:
```bash
chmod 600 creds.conf
```

### 3. Make the Script Executable
Give execution permissions to the main script so your system can run it:
```bash
chmod +x fortinet-login.sh
```

### 4. Create the NetworkManager Dispatcher Script
To make it run automatically every time your Wi-Fi connects, we will tell NetworkManager to trigger it on "up" events for the specific BITS Wi-Fi SSIDs.

Run this entire block exactly as it is in your terminal (while still inside the `bits-wifi-login` directory). It uses `$(pwd)` to automatically bake your current folder's absolute path into the script:

```bash
sudo tee /etc/NetworkManager/dispatcher.d/90-fortinet-login > /dev/null << EOF
#!/usr/bin/env bash

CURRENT_SSID=\$(nmcli -t -f active,ssid dev wifi | grep '^yes' | cut -d: -f2)

if [[ "\$2" == "up" && ( "\$CURRENT_SSID" == "BITS-STUDENT" || "\$CURRENT_SSID" == "BITS-STAFF" ) ]]; then
    sleep 3
    su -c "$(pwd)/fortinet-login.sh >> /tmp/fortinet-login-\$(id -u).log 2>&1 &" $(whoami)
fi
EOF
```

### 5. Make the Dispatcher Executable
NetworkManager will only run files inside `/etc/NetworkManager/dispatcher.d/` if they are executable by root:
```bash
sudo chmod +x /etc/NetworkManager/dispatcher.d/90-fortinet-login
```

You are done! The next time you visit campus and connect to `BITS-STUDENT`, you will be seamlessly logged into the internet within 5 seconds without ever opening a browser.

---

## 🐛 Temporary Files & Debugging

The script specifically uses unique temporary files anchored to your user ID (`$(id -u)`) stored in your `/tmp/` directory to manage cookies securely and preserve logs safely. All of these files are automatically deleted by Linux every time you restart your computer.

* **`/tmp/fortinet-login-$(id -u).log`**: The main log file that tracks every time the script runs (e.g., *[19:30:52] ✓ Login successful!*). Use `cat /tmp/fortinet-login-$(id -u).log` to check output if it stops working.
* **`/tmp/fortinet_cookies_$(id -u).txt`**: Fortinet requires a persistent browser session to link the login page request with the credentials POST form. This file safely stores that temporary web cookie for `curl`.
* **`/tmp/fortinet_error_$(id -u).html`**: If Fortinet rejects your login or expects a specific form field we missed, the script dumps the HTML of the rejection page here so you can read exactly what went wrong.
