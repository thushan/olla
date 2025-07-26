#!/usr/bin/env bash
#########################
# Universal Installer Script v2.0.0
# ---------------------------------
# Simple, lightweight & reusable
#
# <github.com/thushan>
#########################
set -e  # Exit on error

# CONFIGURATION
repo="olla"      # GitHub repo name
name="olla"      # Name of the binary/project
exec="olla"      # Executable name

# Detect if we can use colors, sometimes running in non-tty environments
if [[ -t 1 ]] && [[ -n "$(tput colors 2>/dev/null)" ]] && [[ "$(tput colors)" -ge 8 ]]; then
    CRed=$(tput setaf 1)
    CBlue=$(tput setaf 4)
    CCyan=$(tput setaf 6)
    CGreen=$(tput setaf 2)
    CYellow=$(tput setaf 3)
    BOLD=$(tput bold)
    ENDMARKER=$(tput sgr0)
else
    # No color support
    CRed=""
    CBlue=""
    CCyan=""
    CGreen=""
    CYellow=""
    BOLD=""
    ENDMARKER=""
fi

# Runtime configuration
version="latest"
install_dir=""
show_progress=true
local_file=""

# Allow environment variable overrides for testing
# Example: INSTALLER_REPO=smash ./install.sh
repo="${INSTALLER_REPO:-$repo}"
name="${INSTALLER_NAME:-$name}"
exec="${INSTALLER_EXEC:-$exec}"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            echo "Universal Installer Script v2.0.0"
            echo "github.com/thushan/${repo}"
            echo ""
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  -h, --help              Show this help message"
            echo "  -v, --version VERSION   Install specific version (default: latest)"
            echo "  -d, --dir DIRECTORY     Install to specific directory"
            echo "  -q, --quiet             Quiet mode (no progress)"
            echo "  --no-color              Disable colored output"
            echo "  --local FILE            Use local archive instead of downloading"
            echo ""
            echo "Examples:"
            echo "  $0                      # Install latest version"
            echo "  $0 -v v1.2.3            # Install specific version"
            echo "  $0 -d /usr/local/bin    # Install to specific directory"
            exit 0
            ;;
        -v|--version)
            version="$2"
            shift 2
            ;;
        -d|--dir)
            install_dir="$2"
            shift 2
            ;;
        -q|--quiet)
            show_progress=false
            shift
            ;;
        --no-color)
            CRed=""
            CBlue=""
            CCyan=""
            CGreen=""
            CYellow=""
            BOLD=""
            ENDMARKER=""
            shift
            ;;
        --local)
            local_file="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

echo "----------------------"
echo "${CGreen}${name} Auto-Installer v2.0.0${ENDMARKER}"
echo "${CBlue} <github.com/thushan/${repo}>${ENDMARKER}"
echo "----------------------"

# Cleanup function
cleanup() {
    if [[ -n "$temp_file" ]] && [[ -f "$temp_file" ]]; then
        rm -f "$temp_file"
    fi
}

# Set trap for cleanup
trap cleanup EXIT INT TERM

function fatal() {
    echo ""
    echo "${BOLD}${CRed}FATAL:${ENDMARKER} $1${ENDMARKER}" >&2
    echo ""
    cleanup
    exit 1
}

# Check for required commands
for cmd in curl grep sed; do
    if ! command -v "$cmd" &> /dev/null; then
        fatal "Required command '$cmd' not found. Please install it and try again."
    fi
done

function extract_zip() {
    local filename=$1
    local file_path="${filename%.*}"
    if [[ -x "$(command -v unzip)" ]]; then
        echo "${CCyan}RUN${ENDMARKER} unzip -o ${CYellow}'${filename}'${ENDMARKER}"
        unzip -o "${filename}" -d "${file_path}/"
        if [ $? -eq 0 ]; then
            echo "${CCyan}RAN${ENDMARKER} Extracted ${CGreen}${exec} ${version_display}${ENDMARKER} to ${CYellow}${file_path}/${ENDMARKER}"
            rm -f "${filename}"
            echo "${CCyan}DEL${ENDMARKER} rm ${CYellow}'${filename}'${ENDMARKER}"
        else
            fatal "Failed to extract ${filename}"
        fi
    else
        echo "${BOLD}${CRed}ERROR:${ENDMARKER} unzip not found, please extract manually!${ENDMARKER}" >&2
    fi
}

function extract_tar() {
    local filename=$1
    local file_path="${filename%.*}"
    file_path="${file_path%.tar}"  # Remove .tar from .tar.gz
    
    if [[ -x "$(command -v tar)" ]]; then
        # Create directory first (portable)
        mkdir -p "${file_path}"
        echo "${CCyan}RUN${ENDMARKER} tar -xzf ${CYellow}'${filename}'${ENDMARKER}"
        tar -xzf "${filename}" -C "${file_path}/"
        if [ $? -eq 0 ]; then
            echo "${CCyan}RAN${ENDMARKER} Extracted ${CGreen}${exec} ${version_display}${ENDMARKER} to ${CYellow}${file_path}/${ENDMARKER}"
            rm -f "${filename}"
            echo "${CCyan}DEL${ENDMARKER} rm ${CYellow}'${filename}'${ENDMARKER}"
        else
            fatal "Failed to extract ${filename}"
        fi
    else
        fatal "tar not found, please install it and try again"
    fi
}

if [[ "$OSTYPE" == "linux-gnu"* ]] || [[ "$OSTYPE" == "linux-musl" ]] ; then
  os="linux"
  ext="tar.gz"
  exe=""
elif [[ "$OSTYPE" == "darwin"* ]]; then
  os="macos"
  ext="zip"
  exe=""
elif [[ "$OSTYPE" == "cygwin" ]] || [[ "$OSTYPE" == "msys" ]] || [[ "$OSTYPE" == "win32" ]]; then
  os="windows"
  ext="zip"
  exe=".exe"
elif [[ "$OSTYPE" == "freebsd"* ]]; then
  os="freebsd"
  ext="tar.gz"
  exe=""
else
  fatal "Unsupported OS Type $OSTYPE"
fi

# Detect the architecture
case $(uname -m) in
    x86_64)
        arch="amd64"
        ;;
    arm64|aarch64)
        arch="arm64"
        ;;
    armv7*|armv6*)
        arch="arm"
        ;;
    i386|i686)
        arch="386"
        ;;
    *)
        echo "${CYellow}WARNING:${ENDMARKER} Unknown architecture '$(uname -m)', trying 'amd64'"
        arch="amd64"
        ;;
esac

echo "OS:      ${os}"
echo "ARCH:    ${arch}"

# Get version information
if [[ "$version" == "latest" ]]; then
    latest_release=$(curl --silent --fail "https://api.github.com/repos/thushan/$repo/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -z "$latest_release" ]; then
        fatal "Failed to get the latest release. Please check your internet connection or try again later."
    fi
    version_display="$latest_release"
else
    # Use specified version
    latest_release="$version"
    version_display="$version"
fi

echo "VERSION: ${version_display}"

# Determine install directory
if [[ -z "$install_dir" ]]; then
    install_dir="./${name}_${version_display}_${os}_${arch}"
fi

echo "INSTALL: ${install_dir}"
echo "----------------------"

# Construct the file name (remove 'v' prefix if present for filename)
version_clean="${latest_release#v}"
file="${name}_v${version_clean}_${os}_${arch}.${ext}"
temp_file="$file"

# Construct the download URL
url="https://github.com/thushan/$repo/releases/download/${latest_release}/${file}"

# Handle local file or download
if [[ -n "$local_file" ]]; then
    # Use local file for testing
    echo "${CCyan}USE${ENDMARKER} Local file: ${CBlue}${local_file}${ENDMARKER}"
    if [[ ! -f "$local_file" ]]; then
        fatal "Local file not found: $local_file"
    fi
    cp "$local_file" "$file"
else
    # Download from GitHub
    echo "${CCyan}GET${ENDMARKER} ${CBlue}${url}${ENDMARKER}"
    
    # Download with progress if not quiet
    if [[ "$show_progress" == true ]]; then
        curl_opts="-L"
    else
        curl_opts="-sL"
    fi
    
    # Download the release
    if ! curl $curl_opts "$url" -o "$file"; then
        # Try alternative naming scheme (some repos might use different patterns)
        alt_file="${name}_${latest_release}_${os}_${arch}.${ext}"
        alt_url="https://github.com/thushan/$repo/releases/download/${latest_release}/${alt_file}"
        echo "${CCyan}TRY${ENDMARKER} Alternative URL: ${CBlue}${alt_url}${ENDMARKER}"
        if ! curl $curl_opts "$alt_url" -o "$file"; then
            fatal "Failed to download release. Please check:\n  - Version '${version_display}' exists at github.com/thushan/${repo}/releases\n  - You have internet connectivity"
        fi
    fi
fi

# Check file exists and has size
if [[ ! -f "$file" ]] || [[ ! -s "$file" ]]; then
    fatal "Downloaded file is empty or missing"
fi

file_size=$(du -h "$file" | cut -f1)
file_path="${file%.*}"
if [[ "$ext" == "tar.gz" ]]; then
    file_path="${file_path%.tar}"
fi

# If custom install dir specified, use it
if [[ -n "$install_dir" ]] && [[ "$install_dir" != "./${name}_${version_display}_${os}_${arch}" ]]; then
    file_path="$install_dir"
fi

echo "${CCyan}GOT${ENDMARKER} ${CGreen}${exec} ${version_display}${ENDMARKER} Downloaded ${CYellow}'${file}'${ENDMARKER} ($file_size)"

# Check if already exists and ask to overwrite
if [[ -d "$file_path" ]] && [[ -t 0 ]]; then
    echo "${CYellow}WARNING:${ENDMARKER} Directory ${file_path} already exists."
    read -p "Overwrite? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Installation cancelled."
        cleanup
        exit 0
    fi
    rm -rf "$file_path"
fi

if [[ "$ext" == "zip" ]]; then
    extract_zip "$file"
else
    extract_tar "$file"
fi

# Make binary executable
if [[ -f "${file_path}/${exec}${exe}" ]]; then
    chmod +x "${file_path}/${exec}${exe}"
    echo "${CCyan}YAY${ENDMARKER} Installed ${CGreen}${exec} ${version_display}${ENDMARKER} to ${CYellow}${file_path}/${ENDMARKER}"
    echo ""
    echo "${BOLD}Next steps:${ENDMARKER}"
    echo "  1. ${CGreen}cd ${file_path}${ENDMARKER}"
    echo "  2. ${CGreen}./${exec}${exe}${ENDMARKER}"
    echo ""
    echo "${BOLD}Add to PATH (optional):${ENDMARKER}"
    echo "  ${CGreen}sudo cp ${file_path}/${exec}${exe} /usr/local/bin/${ENDMARKER}"
    echo "  ${BOLD}OR${ENDMARKER}"
    echo "  ${CGreen}export PATH=\"\$PATH:$(pwd)/${file_path}\"${ENDMARKER}"
else
    echo "${CYellow}WARNING:${ENDMARKER} Binary not found at expected location."
    echo "Files in ${file_path}:"
    ls -la "${file_path}/"
fi