# NixOS's /etc is read-only, so the triggers `bits-wifi-login install` writes on
# other distros come from here instead. Same paths and unit names, so `status`
# reads them the same way. Mirrors internal/installer/linux.go: keep in step.
{
  config,
  lib,
  pkgs,
  ...
}: let
  cfg = config.services.bits-wifi-login;
  home = config.users.users.${cfg.user}.home;
  logPath = "${home}/.local/state/bits-wifi-login/dispatcher.log";

  # nm-dispatcher's PATH has neither nmcli nor curl.
  path = lib.makeBinPath (with pkgs; [networkmanager curl gnugrep coreutils util-linux]);

  dispatcher = pkgs.writeScript "90-fortinet-login" ''
    #!${pkgs.bash}/bin/bash
    export PATH=${path}:$PATH
    CURRENT_SSID=$(nmcli -t -f active,ssid dev wifi 2>/dev/null | grep '^yes' | cut -d: -f2)
    if [[ "$2" == "up" || "$2" == "connectivity-change" ]] && [[ "$CURRENT_SSID" =~ ^BITS-(STUDENT|STAFF)$ ]]; then
        tries=0
        until curl -s --max-time 3 -o /dev/null \
            "http://connectivitycheck.gstatic.com/generate_204"; do
            tries=$((tries + 1))
            [[ $tries -ge 10 ]] && exit 0
            sleep 2
        done
        # Redirect inside the user's shell, or root creates the log.
        runuser -u ${cfg.user} -- ${pkgs.bash}/bin/bash -c ${lib.escapeShellArg "${lib.escapeShellArg cfg.executable} >> ${lib.escapeShellArg logPath} 2>&1"}
    fi
  '';

  waitForWifi = pkgs.writeShellScript "bits-wifi-login-wait-for-wifi" ''
    export PATH=${path}:$PATH
    [[ "$(nmcli radio wifi 2>/dev/null)" != "enabled" ]] && exit 0
    tries=0
    while [[ -z "$(nmcli -t -f active,ssid dev wifi 2>/dev/null | grep ^yes | cut -d: -f2)" ]]; do
        tries=$((tries+1))
        [[ $tries -ge 15 ]] && exit 0
        sleep 2
    done
  '';

  sleepTargets = ["suspend.target" "hibernate.target" "hybrid-sleep.target" "suspend-then-hibernate.target"];
in {
  options.services.bits-wifi-login = {
    enable = lib.mkEnableOption "BITS WiFi captive portal auto-login";

    user = lib.mkOption {
      type = lib.types.str;
      description = "User whose binary and creds.conf the triggers run.";
    };

    # creds.conf and session.json live next to the binary, so it stays in a
    # writable home dir rather than the store.
    executable = lib.mkOption {
      type = lib.types.str;
      default = "${home}/.local/share/bits-wifi-login/bits-wifi-login";
      defaultText = lib.literalExpression ''"''${home}/.local/share/bits-wifi-login/bits-wifi-login"'';
      description = "Path to the bits-wifi-login binary that install.sh downloads.";
    };
  };

  config = lib.mkIf cfg.enable {
    networking.networkmanager.enable = lib.mkDefault true;

    environment.etc."NetworkManager/dispatcher.d/90-fortinet-login" = {
      source = dispatcher;
      mode = "0755";
    };

    # `response` must be set whenever `uri` is, see connectivityConf.
    environment.etc."NetworkManager/conf.d/99-bits-wifi-login-connectivity.conf".text = ''
      [connectivity]
      uri=http://connectivitycheck.gstatic.com/generate_204
      response=
      interval=20
    '';

    systemd.tmpfiles.rules = ["d ${home}/.local/state/bits-wifi-login 0700 ${cfg.user} - -"];

    systemd.services.bits-wifi-login = {
      description = "BITS WiFi Fortinet Login";
      after = ["network-online.target"];
      wants = ["network-online.target"];
      path = [pkgs.networkmanager];
      serviceConfig = {
        Type = "oneshot";
        User = cfg.user;
        ExecStart = cfg.executable;
        TimeoutStartSec = 0;
      };
    };

    systemd.services.bits-wifi-login-resume = {
      description = "BITS WiFi Login after resume";
      after = sleepTargets;
      wantedBy = sleepTargets;
      path = [pkgs.networkmanager];
      serviceConfig = {
        Type = "oneshot";
        User = cfg.user;
        ExecStartPre = waitForWifi;
        ExecStart = cfg.executable;
      };
    };

    systemd.timers.bits-wifi-login = {
      description = "BITS WiFi Login periodic check";
      wantedBy = ["timers.target"];
      timerConfig = {
        OnBootSec = "30s";
        OnCalendar = "*:0/10";
        Persistent = true;
      };
    };
  };
}
