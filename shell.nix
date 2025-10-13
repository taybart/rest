{
  pkgs,
  ...
}:
pkgs.mkShell {
  name = "rest";
  buildInputs = with pkgs; [
    go
    gofumpt
    gopls
    gotools
    go-tools
  ];
}
