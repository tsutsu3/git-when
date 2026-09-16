#!/bin/sh
# Installs git-when from GitHub Releases.
# It needs no Go toolchain. It works on Linux, macOS, and Windows shells such as Git Bash.
#
#   sh install.sh                        install the latest release into ~/.local/bin
#   sh install.sh --version v0.1.0       install one release
#   sh install.sh --bin-dir /usr/local/bin
#
# GIT_WHEN_VERSION and BIN_DIR set the same values.

set -eu

REPO="tsutsu3/git-when"
RELEASES="https://github.com/${REPO}/releases"

version="${GIT_WHEN_VERSION:-latest}"
bin_dir="${BIN_DIR:-${HOME}/.local/bin}"
skip_checksum=0
tmp_dir=""

usage() {
	cat <<EOF
Install git-when from GitHub Releases.

Options:
  --version VERSION   release tag to install, for example v0.1.0 (default: latest)
  --bin-dir DIR       directory to install into (default: \$HOME/.local/bin)
  --skip-checksum     do not verify the SHA-256 checksum
  -h, --help          show this help
EOF
}

die() {
	echo "install.sh: $*" >&2
	exit 1
}

have() {
	command -v "$1" >/dev/null 2>&1
}

cleanup() {
	if [ -n "$tmp_dir" ]; then
		rm -rf "$tmp_dir"
	fi
}

# Downloads a URL to a file with curl or wget.
download() {
	if have curl; then
		curl -fsSL "$1" -o "$2"
	elif have wget; then
		wget -q "$1" -O "$2"
	else
		die "curl or wget is required"
	fi
}

# Prints a path in the form that Windows programs understand.
win_path() {
	if have cygpath; then
		cygpath -w "$1"
	else
		printf '%s' "$1"
	fi
}

# Extracts an archive into the temporary directory.
# Git Bash ships GNU tar, and GNU tar cannot read a .zip archive.
# So a .zip needs unzip or the PowerShell that ships with Windows.
extract() {
	case "$1" in
	*.tar.gz)
		tar -xzf "$1" -C "$tmp_dir"
		;;
	*.zip)
		if have unzip; then
			unzip -q "$1" -d "$tmp_dir"
		elif have powershell.exe; then
			powershell.exe -NoProfile -Command \
				"Expand-Archive -LiteralPath '$(win_path "$1")' -DestinationPath '$(win_path "$tmp_dir")' -Force" \
				>/dev/null
		else
			die "unzip or powershell.exe is required to extract a .zip archive"
		fi
		;;
	*) die "unknown archive type: $1" ;;
	esac
}

# Prints the platform part of an archive name, such as linux_amd64.
detect_platform() {
	os="$(uname -s)"
	arch="$(uname -m)"

	case "$os" in
	Linux) os="linux" ;;
	Darwin) os="darwin" ;;
	MINGW* | MSYS* | CYGWIN*) os="windows" ;;
	*) die "unsupported operating system: ${os}" ;;
	esac

	case "$arch" in
	x86_64 | amd64) arch="amd64" ;;
	aarch64 | arm64) arch="arm64" ;;
	*) die "unsupported architecture: ${arch}. Try: go install github.com/${REPO}/cmd/git-when@latest" ;;
	esac

	echo "${os}_${arch}"
}

# Prints the latest release tag.
# The releases page redirects to the tag, so this needs no API token.
resolve_version() {
	if [ "$version" != "latest" ]; then
		echo "$version"
		return
	fi

	url=""
	if have curl; then
		url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "${RELEASES}/latest")"
	elif have wget; then
		url="$(wget -q -S -O /dev/null "${RELEASES}/latest" 2>&1 |
			awk '/[Ll]ocation:/ { last = $2 } END { print last }')"
	else
		die "curl or wget is required"
	fi

	tag="${url##*/}"
	if [ -z "$tag" ] || [ "$tag" = "latest" ]; then
		die "could not find the latest release. Pass --version instead"
	fi
	echo "$tag"
}

# Checks the archive against the checksums file of the same release.
verify_checksum() {
	archive="$1"
	tag="$2"

	if [ "$skip_checksum" -eq 1 ]; then
		return
	fi

	if have sha256sum; then
		sha_check="sha256sum -c -"
	elif have shasum; then
		sha_check="shasum -a 256 -c -"
	else
		die "sha256sum or shasum is required. Use --skip-checksum to install without the check"
	fi

	download "${RELEASES}/download/${tag}/checksums.txt" "${tmp_dir}/checksums.txt"
	line="$(grep "  ${archive}\$" "${tmp_dir}/checksums.txt" || true)"
	if [ -z "$line" ]; then
		die "${archive} is not listed in checksums.txt"
	fi

	# sha256sum reads the file name from the line, so it must run next to the archive.
	(cd "$tmp_dir" && printf '%s\n' "$line" | $sha_check >/dev/null) ||
		die "checksum of ${archive} does not match"
}

while [ $# -gt 0 ]; do
	case "$1" in
	--version)
		[ $# -ge 2 ] || die "--version needs a value"
		version="$2"
		shift 2
		;;
	--bin-dir)
		[ $# -ge 2 ] || die "--bin-dir needs a value"
		bin_dir="$2"
		shift 2
		;;
	--skip-checksum)
		skip_checksum=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*) die "unknown option: $1" ;;
	esac
done

trap cleanup EXIT INT TERM

platform="$(detect_platform)"
tag="$(resolve_version)"
# Archive names carry the version without the leading v.
name="git-when_${tag#v}_${platform}"
case "$platform" in
windows_*)
	archive="${name}.zip"
	exe="git-when.exe"
	;;
*)
	archive="${name}.tar.gz"
	exe="git-when"
	;;
esac

tmp_dir="$(mktemp -d)"

echo "Downloading ${archive} (${tag})"
download "${RELEASES}/download/${tag}/${archive}" "${tmp_dir}/${archive}"
verify_checksum "$archive" "$tag"

extract "${tmp_dir}/${archive}"
if [ ! -f "${tmp_dir}/${name}/${exe}" ]; then
	die "the archive does not contain ${exe}"
fi

mkdir -p "$bin_dir"
install -m 755 "${tmp_dir}/${name}/${exe}" "${bin_dir}/${exe}" 2>/dev/null ||
	{ cp "${tmp_dir}/${name}/${exe}" "${bin_dir}/${exe}" && chmod 755 "${bin_dir}/${exe}"; }

echo "Installed git-when ${tag} in ${bin_dir}"

case ":${PATH}:" in
*":${bin_dir}:"*) ;;
*) echo "Add ${bin_dir} to your PATH to run git-when." ;;
esac
