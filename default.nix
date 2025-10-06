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
  vendorHash = "sha256-3bIQvvzfWxOnRCHAwZ1RCjq2ieXNtBkPr4hGY1q1798=";

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
