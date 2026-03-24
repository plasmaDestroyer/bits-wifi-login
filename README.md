# BITS Wifi Login Script

After cloning the repo, run these commands(replace the path with the correct path):

```bash
sudo tee /etc/NetworkManager/dispatcher.d/90-fortinet-login > /dev/null << EOF
#!/usr/bin/env bash

CURRENT_SSID=\$(nmcli -t -f active,ssid dev wifi | grep '^yes' | cut -d: -f2)

if [[ "\$2" == "up" && \("\$CURRENT_SSID" == "BITS-STUDENT" || "\$CURRENT_SSID" == "BITS-STAFF"\) ]]; then
    sleep 3
    su -c "/home/$(whoami)/path/to/bits-wifi-login/fortinet-login.sh >> /tmp/fortinet-login.log 2>&1 &" $(whoami)
fi
EOF
```
then make it executable:

```bash
sudo chmod +x /etc/NetworkManager/dispatcher.d/90-fortinet-login
```
