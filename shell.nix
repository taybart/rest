{
  pkgs,
  gomod2nix,
  ...
}:
pkgs.mkShell {
  name = "rest";
  buildInputs = with pkgs; [
    go
    gomod2nix
    gofumpt
    gopls
    gotools
    go-tools
  ];
}
