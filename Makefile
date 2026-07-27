PLUGIN_NAME := balance-query
OUTPUT_DIR  := bin
OUTPUT      := $(OUTPUT_DIR)/$(PLUGIN_NAME).so

GO      ?= go
GOFLAGS := -buildmode=c-shared -trimpath
LDFLAGS := -ldflags="-s -w"

.PHONY: all build clean fmt test vet

all: build

build:
	@mkdir -p $(OUTPUT_DIR)
	CGO_ENABLED=1 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(OUTPUT) .
	@echo "Built: $(OUTPUT)"

clean:
	@rm -rf $(OUTPUT_DIR)

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...
