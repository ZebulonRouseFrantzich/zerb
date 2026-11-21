{
  description = "ZERB - Zero-hassle Effortless Reproducible Builds";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    chezmoi-zerb = {
      url = "github:ZebulonRouseFrantzich/chezmoi/ef897f7418ba0bd5423a5bf1563d45f05d9c6ea5";
      flake = false;
    };
  };

  outputs = { self, nixpkgs, flake-utils, chezmoi-zerb }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages.chezmoi-zerb = pkgs.buildGoModule {
          pname = "chezmoi-zerb";
          version = "2.70.5-zerb.1";
          src = chezmoi-zerb;
          vendorHash = "sha256-jCKO/ih/HRLZ5hv4Ib8qB9kIJagJQq79wtSqBi+BgCY=";
          env.CGO_ENABLED = "0";
          subPackages = [ "." ];
          ldflags = [
            "-s"
            "-w"
            "-X main.version=v2.70.5-zerb.1"
            "-X main.commit=ef897f7418ba0bd5423a5bf1563d45f05d9c6ea5"
            "-X main.date=1787510541"
            "-X main.builtBy=zerb-phase1-proof"
          ];
          doCheck = true;
          checkPhase = ''
            runHook preCheck
            go test ./internal/cmd -run '^TestManagedCmd(SkipTemplateDataEnvironment)?$' -count=1
            runHook postCheck
          '';
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Core Go Development (1.21+ required)
            go           # Latest stable Go
            gotools      # includes goimports
            gopls        # Go language server

            # Code Quality
            golangci-lint
            golines      # Line length formatter
            gofumpt      # Stricter gofmt

            # Testing & Coverage
            gotestsum         # Better test output
            go-junit-report   # CI integration

            # Task Runner
            just         # Command runner (Justfile)

            # Development Utilities
            direnv       # Auto-load environment
            ripgrep      # Fast code search
            fd           # Fast file finding
            jq           # JSON processing
          ];

          shellHook = ''
            # Display welcome message
            echo "╔══════════════════════════════════════════════════════════╗"
            echo "║  ZERB Development Environment                            ║"
            echo "║  Zero-hassle Effortless Reproducible Builds              ║"
            echo "╚══════════════════════════════════════════════════════════╝"
            echo ""
            echo "Go version: $(go version | cut -d' ' -f3,4)"
            echo "Project: ZERB v0.1.0-alpha"
            echo ""
            echo "📋 Available Commands (via Justfile):"
            echo "  just test         - Run all tests"
            echo "  just lint         - Run linters"
            echo "  just build        - Build binary"
            echo "  just coverage     - Generate coverage report"
            echo "  just fmt          - Format code"
            echo "  just vet          - Run Go vet"
            echo ""
            echo "📚 Documentation:"
            echo "  https://github.com/ZebulonRouseFrantzich/zerb#readme"
            echo ""

            # Create a temporary file in the current directory or a temp directory
            # For simplicity, we create it in the current directory as a hidden file.
            echo "--no-ignore-vcs" > .rgignore_config
            export RIPGREP_CONFIG_PATH="$PWD/.rgignore_config"

            # Set up Go environment
            export GOPATH="$HOME/go"
            export PATH="$GOPATH/bin:$PATH"

            # Project-specific environment variables
            export ZERB_DEV=1
            export ZERB_TEST_MODE=1

            # Prevent ZERB tests from interfering with system
            export ZERB_TEST_ROOT="$PWD/.test-tmp"
            mkdir -p "$ZERB_TEST_ROOT"

            # Aliases for convenience
            alias t="just test"
            alias b="just build"
            alias l="just lint"
          '';
        };
      }
    );
}
