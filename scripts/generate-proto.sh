#!/bin/bash

# Copyright 2025 The Ceph-CSI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Script to generate protobuf Go stubs from ceph-nvmeof project
# This script uses committed .proto files and generates Go stubs for gRPC communication

set -euo pipefail

# Source build environment for versions
if [[ -f "build.env" ]]; then
    source build.env
else
    echo "[ERROR] build.env not found. Please run from project root."
    exit 1
fi

# Configuration
TARGET_DIR="internal/nvme/gateway/proto"
GENERATED_DIR="${TARGET_DIR}"

# Logging prefixes
INFO_PREFIX="[INFO]"
ERROR_PREFIX="[ERROR]"
WARN_PREFIX="[WARN]"

# Function to log messages
log_info() {
    echo "${INFO_PREFIX} $1"
}

log_error() {
    echo "${ERROR_PREFIX} $1" >&2
}

log_warn() {
    echo "${WARN_PREFIX} $1"
}

# Function to check if protoc is available
check_protoc() {
    if ! command -v protoc >/dev/null 2>&1; then
        log_error "protoc not found. Please install protobuf-compiler."
        exit 1
    fi

    local protoc_version
    protoc_version=$(protoc --version | cut -d' ' -f2)
    log_info "Using protoc version: ${protoc_version}"
}

# Function to check if Go plugins are available
check_go_plugins() {
    local missing_plugins=()

    # Add Go bin directory to PATH if not already there
    if [[ -n "${GOPATH:-}" ]]; then
        export PATH="${GOPATH}/bin:${PATH}"
        log_info "Added ${GOPATH}/bin to PATH"
    elif [[ -d "${HOME}/go/bin" ]]; then
        export PATH="${HOME}/go/bin:${PATH}"
        log_info "Added ${HOME}/go/bin to PATH"
    fi

    if ! command -v protoc-gen-go >/dev/null 2>&1; then
        missing_plugins+=("protoc-gen-go")
    fi

    if ! command -v protoc-gen-go-grpc >/dev/null 2>&1; then
        missing_plugins+=("protoc-gen-go-grpc")
    fi

    if [[ ${#missing_plugins[@]} -gt 0 ]]; then
        log_error "Missing Go protobuf plugins: ${missing_plugins[*]}"
        log_error "Please install: go install google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION}"
        log_error "Please install: go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION}"
        exit 1
    fi

    log_info "Go protobuf plugins found"
}



# Function to add go_package option if missing
add_go_package_option() {
    local proto_file="$1"
    local package_name

    # Extract package name from filename
    package_name=$(basename "${proto_file}" .proto)

    # Check if go_package option already exists
    if ! grep -q "option go_package" "${proto_file}"; then
        log_info "Adding go_package option to ${proto_file}"

        # Add go_package option after the syntax declaration
        sed -i '/^syntax = "proto3";/a option go_package = "github.com/ceph/ceph-csi/internal/nvme/gateway/proto;'"${package_name}"'";' "${proto_file}"

        log_info "Successfully added go_package option"
    else
        log_info "go_package option already exists in ${proto_file}"
    fi
}

# Function to generate Go stubs
generate_go_stubs() {
    local proto_file="$1"
    local proto_path
    proto_path="${TARGET_DIR}/$(basename "${proto_file}")"

    log_info "Generating Go stubs from ${proto_path}..."

    # Ensure we're in the project root for proper import paths
    cd "$(dirname "$0")/.."

    # Generate Go stubs with proper flags
    if protoc \
        --experimental_allow_proto3_optional \
        --proto_path="${TARGET_DIR}" \
        --go_out="${GENERATED_DIR}" \
        --go_opt=paths=source_relative \
        --go-grpc_out="${GENERATED_DIR}" \
        --go-grpc_opt=paths=source_relative \
        "${proto_path}"; then

        log_info "Successfully generated Go stubs for ${proto_file}"
    else
        log_error "Failed to generate Go stubs for ${proto_file}"
        exit 1
    fi
}

# Function to process a single proto file
process_proto_file() {
    local proto_file="$1"

    log_info "Processing ${proto_file}..."

    # Add go_package option if needed
    local target_file
    target_file="${TARGET_DIR}/$(basename "${proto_file}")"
    add_go_package_option "${target_file}"

    # Generate Go stubs
    generate_go_stubs "${proto_file}"

    log_info "Successfully processed ${proto_file}"
}

# Function to clean up generated files
cleanup_generated_files() {
    log_info "Cleaning up generated files..."

    if [[ -d "${GENERATED_DIR}" ]]; then
        find "${GENERATED_DIR}" -name "*.pb.go" -type f -exec rm -f {} \;
        log_info "Cleaned up generated Go files"
    fi
}

# Function to show usage
show_usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Generate protobuf Go stubs from ceph-nvmeof project.

OPTIONS:
    -h, --help          Show this help message
    -c, --clean         Clean up generated files only
    -f, --force         Force regeneration (clean first)
    -v, --version       Show version information

ENVIRONMENT:
    Uses versions from build.env:
    - PROTOC_VERSION: ${PROTOC_VERSION}
    - PROTOC_GEN_GO_VERSION: ${PROTOC_GEN_GO_VERSION}
    - PROTOC_GEN_GO_GRPC_VERSION: ${PROTOC_GEN_GO_GRPC_VERSION}



EXAMPLES:
    $0                    # Generate protobuf stubs
    $0 --clean           # Clean up generated files
    $0 --force           # Force regeneration
EOF
}

# Function to show version
show_version() {
    echo "generate-proto.sh version 1.0"
    echo "Protobuf tool versions:"
    echo "  protoc: ${PROTOC_VERSION}"
    echo "  protoc-gen-go: ${PROTOC_GEN_GO_VERSION}"
    echo "  protoc-gen-go-grpc: ${PROTOC_GEN_GO_GRPC_VERSION}"
}

# Main execution
main() {
    local clean_only=false
    local force_regeneration=false

    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_usage
                exit 0
                ;;
            -c|--clean)
                clean_only=true
                shift
                ;;
            -f|--force)
                force_regeneration=true
                shift
                ;;
            -v|--version)
                show_version
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done

    # Change to script directory for relative paths
    cd "$(dirname "$0")/.."

    log_info "Starting protobuf generation process..."

    # Check dependencies
    check_protoc
    check_go_plugins

    # Show Go version
    local go_version
    go_version=$(go version | cut -d' ' -f3)
    log_info "Using Go version: ${go_version}"

    # Handle clean operation
    if [[ "${clean_only}" == true ]]; then
        cleanup_generated_files
        log_info "Cleanup completed"
        exit 0
    fi

    # Handle force regeneration
    if [[ "${force_regeneration}" == true ]]; then
        log_info "Force regeneration requested, cleaning first..."
        cleanup_generated_files
    fi

    # Process the proto file
    process_proto_file "gateway.proto"

    log_info "Protobuf generation completed successfully!"
    log_info "Generated files are available in: ${GENERATED_DIR}"
}

# Run main function with all arguments
main "$@"
