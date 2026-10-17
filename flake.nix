{
  description = "BITS WiFi captive portal auto-login";

  outputs = _: {
    nixosModules.default = import ./nix/module.nix;
  };
}
