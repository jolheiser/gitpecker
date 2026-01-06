{
  description = "git + woodpecker";
  inputs.nixpkgs.url = "github:nixos/nixpkgs/nixpkgs-unstable";
  outputs =
    {
      self,
      nixpkgs,
      ...
    }:
    let
      systems = [
        "aarch64-darwin"
        "aarch64-linux"
        "x86_64-darwin"
        "x86_64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems f;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          default = self.packages.${system}.gitpecker;
          gitpecker = pkgs.buildGoModule rec {
            pname = "gitpecker";
            version = self.rev or "dev";
            src = pkgs.nix-gitignore.gitignoreSource [ ] (
              builtins.path {
                name = pname;
                path = ./.;
              }
            );
            vendorHash = nixpkgs.lib.fileContents ./go.mod.sri;
            meta = {
              description = "git + woodpecker";
              homepage = "https://github.com/jolheiser/gitpecker";
              mainProgram = "gitpecker";
            };
          };
        }
      );
      devShells = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          default = pkgs.mkShell {
            nativeBuildInputs = with pkgs; [
              go
              gopls
              gofumpt
              woodpecker-server
            ];
          };
        }
      );
      nixosConfigurations.gitpeckerVM = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [
          (
            { pkgs, ... }:
            {
              services.getty.autologinUser = "root";
              services.woodpecker-server = {
                enable = true;
                package = pkgs.woodpecker-server.overrideAttrs (oldAttrs: {
                  doCheck = false;
                  patches = (oldAttrs.patches or [ ]) ++ [ ./debug.patch ];
                });
                environment = {
                  WOODPECKER_HOST = "http://0.0.0.0:8000";
                  WOODPECKER_OPEN = "true";
                  WOODPECKER_ADDON_FORGE = "${self.packages.x86_64-linux.gitpecker}/bin/gitpecker";
                  WOODPECKER_LOG_LEVEL = "trace";
                  WOODPECKER_ADMIN = "jolheiser";
                  WOODPECKER_AGENT_SECRET = "Testing123";
                  GITPECKER_REPOS = "/var/lib/forge/";
                  GITPECKER_URL = "/var/lib/forge/";
                  GITPECKER_PROVIDER = "";
                  GITPECKER_CLIENT_ID = "";
                  GITPECKER_CLIENT_SECRET = "";
                  GITPECKER_REDIRECT = "http://localhost:8000/authorize";
                  GITPECKER_LOG_FILE = "/var/lib/woodpecker-server/addon.log";
                  GITPECKER_LOG_LEVEL = "debug";
                };
              };
              services.woodpecker-agents.agents."007" = {
                enable = true;
                path = with pkgs; [
                  git
                  git-lfs
                  bash
                  coreutils
                  woodpecker-plugin-git
                ];
                environment = {
                  WOODPECKER_AGENT_SECRET = "Testing123";
                };
              };
              virtualisation.vmVariant.virtualisation = {
                cores = 2;
                memorySize = 2048;
                graphics = false;
              };
              networking.firewall.enable = false;
              systemd.services."setup-vm" = {
                wantedBy = [ "multi-user.target" ];
                path = with pkgs; [
                  git
                ];
                serviceConfig = {
                  Type = "oneshot";
                  RemainAfterExit = true;
                  User = "root";
                  Group = "root";
                  ExecStart =
                    let
                      pipeline = {
                        steps = [
                          {
                            name = "honk";
                            image = "bash";
                            commands = [ ''echo "honk!"'' ];
                          }
                        ];
                      };
                    in
                    pkgs.writeShellScript "setup-vm-script" ''
                      git config --global user.name "NixUser"
                      git config --global user.email "nixuser@example.com"
                      git config --global init.defaultBranch main
                      git config --global push.autoSetupRemote true
                              
                      mkdir -p /var/lib/forge
                      pushd /var/lib/forge
                      git init --bare test1.git
                      git init --bare test2.git
                      popd
                      git clone /var/lib/forge/test1.git
                      pushd test1
                      cp ${(pkgs.formats.yaml { }).generate "pipeline.yaml" pipeline} .woodpecker.yaml
                      git add .
                      git commit -m "honk"
                      git push
                      popd
                    '';
                };
              };
              environment.systemPackages = with pkgs; [
                git
              ];
              system.stateVersion = "23.11";
            }
          )
        ];
      };
      apps = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          vm = {
            type = "app";
            program = "${pkgs.writeShellScript "vm" ''
              nixos-rebuild build-vm --flake .#gitpeckerVM
              export QEMU_NET_OPTS="hostfwd=tcp::8000-:8000"
              ./result/bin/run-nixos-vm
              rm nixos.qcow2
            ''}";
          };
        }
      );
    };
}
