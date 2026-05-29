{
  description = "Upload files and directories to unique, expiring paths in Cloudflare R2";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages = {
          dollop = pkgs.buildGoModule {
            pname = "dollop";
            version = "1.5.0"; # x-release-please-version

            src = ./.;

            vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";

            subPackages = [ "cmd/dollop" ];

            meta = {
              description = "Upload files and directories to unique, expiring paths in Cloudflare R2";
              homepage = "https://github.com/jamestelfer/dollop";
              license = pkgs.lib.licenses.asl20;
              mainProgram = "dollop";
            };
          };

          default = self.packages.${system}.dollop;
        };
      });
}
