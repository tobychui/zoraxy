{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixpkgs-unstable";

    flake-parts = {
      url = "github:hercules-ci/flake-parts";
      inputs.nixpkgs-lib.follows = "nixpkgs";
    };
  };

  outputs = { ... } @ inputs:
  inputs.flake-parts.lib.mkFlake { inherit inputs; } {
    systems = [ "x86_64-linux" ];

    perSystem = { self', system, ... }:
    let
      pkgs = import inputs.nixpkgs { inherit system; };
      lib = pkgs.lib;
    in
    {
      devShells = {
        default = pkgs.mkShell {
          packages = with pkgs; [
            act dive trivy
          ];
          shellHook = ''
            alias nr="nix run .#"
          '';
        };
      };
      packages = let
        build = lib.getExe self'.packages.build;
      in {
        default = self'.packages.test;
        build = pkgs.writeShellApplication {
          name = "build";
          runtimeInputs = with pkgs; [ docker ];
          text = ''
            cp -lr ../src/ ./
            docker build . -t test-zoraxy --cache-from zoraxydocker/zoraxy:latest ;\
            rm -r ./src/
          '';
          meta.mainProgram = "build";
        };
        test = pkgs.writeShellApplication {
          name = "test-container";
          runtimeInputs = with pkgs; [ docker ];
          text = ''
            ${build}
            docker compose -f ./docker-compose-test.yaml up
          '';
        };
        trivy = pkgs.writeShellApplication {
          name = "trivy-image";
          runtimeInputs = with pkgs; [ docker trivy ];
          text = ''
            ${build}
            trivy image test-zoraxy
          '';
        };
      };
    };
  };
}

