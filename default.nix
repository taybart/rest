{
  pkgs,
  self,
  buildGoApplication,
  version ? "0.0.1",
  ...
}:
buildGoApplication {
  pname = "rest";
  inherit version;
  pwd = self;
  src = self;
  modules = ./gomod2nix.toml;
  CGO_ENABLED = "0";

  subPackages = [
    "cmd/rest"
  ];

  meta = with pkgs.lib; {
    mainProgram = "rest";
    description = "rest easy";
    homepage = "https://github.com/taybart/rest";
    # license = with licenses; [ ];
    maintainers = with maintainers; [ jacbart ];
    platforms = platforms.unix;
  };
}
