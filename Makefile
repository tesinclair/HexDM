.PHONY: all help deps build build_auth run demo test install clean

BIN_NAME=hexdm
HELPER_BIN_NAME=hex-auth

INSTALL_DIR=/usr/local/bin
PAM_DIR=/etc/pam.d
SYSTEMD_DIR=/etc/systemd/system
HELPER_DIR=/usr/libexec/hexdm
CONFIG_DIR=/etc/hexdm

all: help

help:
	@echo "HexDM"
	@echo "Hex Display Manager is a rendered, keyboard driven display manager written in Go for wayland systems."
	@echo ""
	@echo "NORMAL USAGE:"
	@echo " sudo make install   : Compiles and installs the program"
	@echo ""
	@echo "SPECIAL USAGE: "
	@echo "	make build          : Compiles the binary to ./bin/hex"
	@echo "	sudo make install   : Installs binary, PAM configs, and systemd service"
	@echo "	make clean          : Removes build artifacts"
	@echo "	sudo make run       : Runs locally for testing (requires sudo for PAM)"
	@echo "	make demo           : Shows the greeter in a window with a pretend system behind it (no root, no PAM)"
	@echo "	make test           : Runs the tests"

# The rendered greeter draws with SDL3 and OpenGL ES.
deps:
	@pkg-config --exists sdl3 glesv2 || { \
		echo "Error: SDL3 and OpenGL ES development files are needed."; \
		echo "  Arch:   pacman -S sdl3 mesa"; \
		echo "  Fedora: dnf install SDL3-devel mesa-libGLES-devel"; \
		echo "  Debian: apt install libsdl3-dev libgles-dev"; \
		exit 1; \
	}

build_auth:
	@mkdir -p bin/
	@echo "Building Hex Authenticator..."
	CGO_ENABLED=1 go build -o bin/hex-auth cmd/hexdm-auth/main.go
	@echo "Build complete: bin/hex-auth"

build: deps
	@mkdir -p bin/
	@echo "Building Hex Display Manager"
	CGO_ENABLED=1 go build -o bin/hexdm ./cmd/hexdm
	@echo "Build complete: bin/hex"

run: build_auth
	@if [ ! -f /etc/pam.d/hexdm ]; then \
		echo "Error: PAM config missing. Run 'sudo make install' first."; \
		exit 1; \
	fi
	@echo "Running HexDM (Debug Mode)..."
	@mkdir -p logs/
	HEX_DEBUG=1 HEX_DRYRUN=1 HEX_FPRINT_PATH=bin/hex-auth go run cmd/hexdm/main.go 2> logs/crash.log
	@echo "--- CRASH LOG ---"
	@cat logs/crash.log

# No root and no PAM: made-up users, and nothing is ever started. Log in with
# the password "hex". Try DEMO_ARGS="-background starfield" (or -tui).
demo: deps
	CGO_ENABLED=1 go run ./cmd/hexdm-demo $(DEMO_ARGS)

test: deps
	CGO_ENABLED=1 go test ./...

install: build build_auth
	@if [ "$$(id -u)" -ne 0 ]; then echo "Must be root. (sudo make install)"; exit 1; fi
	
	@echo "Installing Binaries..."
	install -D -m 755 bin/$(BIN_NAME) $(INSTALL_DIR)/$(BIN_NAME)
	install -D -m 755 bin/$(HELPER_BIN_NAME) $(HELPER_DIR)/$(HELPER_BIN_NAME)
	@echo "done."
	
	@echo "Installing Configs..."
	install -D -m 644 config/hexdm $(PAM_DIR)/hexdm
	install -D -m 644 config/hexdm_nofprint $(PAM_DIR)/hexdm_nofprint
	install -D -m 644 config/hexdm.service $(SYSTEMD_DIR)/hexdm.service
	@if [ ! -f $(CONFIG_DIR)/hexdm.conf ]; then \
		install -D -m 644 config/hexdm.conf $(CONFIG_DIR)/hexdm.conf; \
	else \
		echo "Keeping your existing $(CONFIG_DIR)/hexdm.conf"; \
	fi
	@echo "done."
	
	@echo "Installing Systemd Service..."
	@cp config/hexdm.service $(SYSTEMD_DIR)/hexdm.service
	@echo "done."
	
	@echo "Enabling Service..."
	systemctl daemon-reload
	systemctl enable hexdm
	@systemctl set-default graphical.target
	@echo "done."
	
	@echo ""
	@echo "--- INSTALLATION COMPLETE ---"
	@echo "Finished. Reboot to start HexDM."

clean:
	@echo "Cleaning..."
	@rm -rf bin/ logs/
	@go clean
