{
  pkgs,
  self,
  version ? "0.0.1",
  ...
}:
pkgs.buildGoModule rec {
  pname = "rest";
  inherit version;
  src = self;
  vendorHash = "sha256-lX69/KtXnsZ7ZPUq38JlccfeeT4BGD3OJ7AYMFK4M/A=";

  env = {
    CGO_ENABLED = "0";
  };

  subPackages = [
    "cmd/rest"
  ];

  meta = with pkgs.lib; {
    mainProgram = pname;
    description = "rest easy";
    homepage = "https://github.com/taybart/rest";
    # license = with licenses; [ ];
    maintainers = with maintainers; [ jacbart ];
    platforms = platforms.unix;
  };
}
