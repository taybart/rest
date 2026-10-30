{
  description = "rest flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    gomod2nix.url = "github:nix-community/gomod2nix";
    gomod2nix.inputs.nixpkgs.follows = "nixpkgs";
  };
  outputs =
    {
      nixpkgs,
      gomod2nix,
      self,
      ...
    }:
    let
      allSystems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems =
        f:
        nixpkgs.lib.genAttrs allSystems (
          system:
          f {
            pkgs = import nixpkgs { inherit system; };
            inherit system;
          }
        );
      version = "0.10.2";
    in
    {
      # nix run/build
      packages = forAllSystems (
        { pkgs, system }:
        {
          default = pkgs.callPackage ./default.nix {
            inherit version self;
            buildGoApplication = gomod2nix.legacyPackages.${system}.buildGoApplication;
          };
          # nix run .#gomod2nix -- generate
          gomod2nix = gomod2nix.legacyPackages.${system}.gomod2nix;
        }
      );
      # nix develop
      devShells = forAllSystems (
        { pkgs, system }:
        {
          default = pkgs.callPackage ./shell.nix {
            gomod2nix = gomod2nix.legacyPackages.${system}.gomod2nix;
          };
        }
      );
      # nix fmt
      formatter = nixpkgs.lib.genAttrs allSystems (system: nixpkgs.legacyPackages.${system}.nixfmt-tree);
    };
}
