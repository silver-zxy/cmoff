{
  description = "Local server and client hook for Clash Mini 1.1143.2 (arm64)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-24.05";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfree = true;
          config.android_sdk.accept_license = true;
        };

        android = pkgs.androidenv.composeAndroidPackages {
          includeNDK = true;
          ndkVersions = [ "26.1.10909125" ];
          buildToolsVersions = [ "34.0.0" ];
          platformVersions = [ "34" ];
        };
        sdk = "${android.androidsdk}/libexec/android-sdk";

        tools = [ pkgs.go pkgs.python3 pkgs.patchelf pkgs.jdk17 pkgs.coreutils ];

        # NDK clang and apksigner, globbed so a version bump needs no edit.
        toolenv = ''
          NDK="$(echo ${sdk}/ndk/*)"
          export NDK
          PATH="$(echo ${sdk}/build-tools/*):$PATH"
          export PATH
        '';

        # build.sh does the work; the flake only supplies the tools.
        mkApp = mode: pkgs.writeShellApplication {
          name = "cmoff-${mode}";
          runtimeInputs = tools;
          text = ''
            ${toolenv}
            export CMOFF_ROOT=${self}
            exec bash ${self}/build.sh ${mode} "$@"
          '';
        };
      in {
        apps.standalone = { type = "app"; program = "${mkApp "standalone"}/bin/cmoff-standalone"; };
        apps.connect = { type = "app"; program = "${mkApp "connect"}/bin/cmoff-connect"; };
        apps.server = { type = "app"; program = "${mkApp "server"}/bin/cmoff-server"; };
        apps.default = self.apps.${system}.standalone;

        devShells.default = pkgs.mkShell {
          packages = tools ++ [ android.androidsdk ];
          shellHook = toolenv + ''
            echo "cmoff dev shell: go, \$NDK, apksigner, patchelf; run ./build.sh"
          '';
        };
      });
}
