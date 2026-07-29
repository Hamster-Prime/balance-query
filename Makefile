PLUGIN_NAME ?= balance-query
OUTPUT_DIR  ?= bin

GO           ?= go
GOOS         ?= $(shell $(GO) env GOOS)
GOARCH       ?= $(shell $(GO) env GOARCH)
CGO_ENABLED  ?= 1

ifeq ($(GOOS),windows)
SHARED_LIB_EXT := dll
else ifeq ($(GOOS),darwin)
SHARED_LIB_EXT := dylib
else
SHARED_LIB_EXT := so
endif

OUTPUT  ?= $(OUTPUT_DIR)/$(PLUGIN_NAME).$(SHARED_LIB_EXT)
BUILD_FLAGS := -buildmode=c-shared -trimpath
LDFLAGS     := -ldflags="-s -w"

.PHONY: all build clean fmt test vet

all: build

build:
	@mkdir -p $(dir $(OUTPUT))
	CC=$(CC) CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build $(BUILD_FLAGS) $(LDFLAGS) -o $(OUTPUT) .
	@echo "Built: $(OUTPUT)"

clean:
	@rm -f $(OUTPUT_DIR)/$(PLUGIN_NAME).so $(OUTPUT_DIR)/$(PLUGIN_NAME).dylib $(OUTPUT_DIR)/$(PLUGIN_NAME).dll $(OUTPUT_DIR)/$(PLUGIN_NAME).h

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...
