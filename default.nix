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
  vendorHash = "sha256-aIAQs+k/nuJAWYFw5H3Rq7w0ZiyADcun3BsreV/VB/I=";

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
