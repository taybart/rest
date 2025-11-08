{
  description = "rest flake";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };
  outputs =
    {
      nixpkgs,
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
          }
        );
      version = "0.7.0";
    in
    {
      # nix run/build
      packages = forAllSystems (
        { pkgs }:
        {
          default = pkgs.callPackage ./default.nix { inherit self pkgs version; };
        }
      );
      # nix develop
      devShells = forAllSystems (
        { pkgs }:
        {
          default = pkgs.callPackage ./shell.nix { };
        }
      );
      # nix fmt
      formatter = nixpkgs.lib.genAttrs allSystems (system: nixpkgs.legacyPackages.${system}.nixfmt-tree);
    };
}
