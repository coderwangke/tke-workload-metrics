PROJECT_NAME := tke-workload-metrics

# Supported platforms
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64

# Output directory
BUILD_DIR := bin

# Default target
.PHONY: all
all: build

# Build the project for all platforms
.PHONY: build
build: $(PLATFORMS)
	@echo "Build completed for all platforms."

# Platform-specific build
$(PLATFORMS):
	@GOOS=$(word 1,$(subst /, ,$@)) GOARCH=$(word 2,$(subst /, ,$@)) \
	go build -o $(BUILD_DIR)/$(PROJECT_NAME)_$(word 1,$(subst /, ,$@))_$(word 2,$(subst /, ,$@)) ./...

# Clean up the build artifacts
.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
	@echo "Cleaned up build artifacts."