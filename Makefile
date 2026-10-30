PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: build run test lint clean install uninstall check-deps

check-deps:
ifeq ($(shell uname -s),Linux)
	@echo "Checking dependencies..."
	@pkg-config --exists gstreamer-1.0 2>/dev/null || \
		{ echo ""; \
		echo "ERROR: gstreamer-1.0 not found."; \
		echo "Please install: gstreamer, gstreamer-plugins-base, gstreamer-plugins-good, gst-libav (an AAC decoder is required)"; \
		echo ""; exit 1; }
	@echo "All dependencies found."
endif

# build compiles aux. Last.fm API keys are embedded when LASTFM_API_KEY and
# LASTFM_API_SECRET are set in the environment or a local .env (gitignored);
# without them the user sets lastfm_api_key and lastfm_api_secret in the config.
build: check-deps
	@set -e; \
	if [ -f .env ]; then set -a; . ./.env; set +a; fi; \
	ldflags=""; \
	if [ -n "$$LASTFM_API_KEY" ] && [ -n "$$LASTFM_API_SECRET" ]; then \
		echo "Embedding Last.fm API keys..."; \
		ldflags="-X 'github.com/oglenyaboss/aux-player/internal/lastfm.apiKey=$$LASTFM_API_KEY' -X 'github.com/oglenyaboss/aux-player/internal/lastfm.apiSecret=$$LASTFM_API_SECRET'"; \
	fi; \
	if [ -n "$$ldflags" ]; then \
		go build -ldflags "$$ldflags" -o aux .; \
	else \
		go build -o aux .; \
	fi

# install builds aux and copies the binary onto PATH, so the installed copy
# cannot silently go stale behind a newer ./aux in the working tree. No Apple
# credentials are needed: aux fetches its developer token from music.apple.com
# at startup. Override the location with: make install PREFIX=/usr/local
install: build
	install -d "$(BINDIR)"
	install -m 755 aux "$(BINDIR)/aux"
	@echo "Installed aux to $(BINDIR)/aux"

uninstall:
	rm -f "$(BINDIR)/aux"

run: build
	./aux

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -f aux
