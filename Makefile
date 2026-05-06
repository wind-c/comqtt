TAILWIND_VERSION ?= v3.4.7
TAILWIND_BIN := bin/tailwindcss

.PHONY: dashboard
dashboard: $(TAILWIND_BIN)
	$(TAILWIND_BIN) -c dashboard/web/tailwind.config.js \
		-i dashboard/web/input.css \
		-o dashboard/static/tailwind.css --minify

$(TAILWIND_BIN):
	@mkdir -p bin
	@case "$$(uname -s)-$$(uname -m)" in \
		Darwin-arm64) URL="https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-macos-arm64";; \
		Darwin-x86_64) URL="https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-macos-x64";; \
		Linux-x86_64) URL="https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-linux-x64";; \
		Linux-aarch64) URL="https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-linux-arm64";; \
		*) echo "unsupported platform: $$(uname -s)-$$(uname -m)" >&2; exit 1;; \
	esac; \
	curl -L -o $@ $$URL && chmod +x $@
