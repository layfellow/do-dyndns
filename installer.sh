#!/bin/sh

set -eu

base_url="https://github.com/layfellow/do-dyndns/releases/latest/download"
binary="do-dyndns"

main() {
	os=$( uname -s )
	arch=$( uname -m )

	case "$os-$arch" in
		Darwin-arm64)
			url="$base_url/$binary-darwin-arm64"
			;;
		Darwin-x86_64)
			url="$base_url/$binary-darwin-amd64"
			;;
		Linux-x86_64)
			url="$base_url/$binary-linux-amd64"
			;;
		*)
			printf "installer: sorry, native Windows is not supported.\n" >&2
			exit 1
			;;
	esac

	if command -v curl >/dev/null 2>&1; then
		fetch_cmd="curl -sSL -o $binary \"$url\""
	elif command -v wget >/dev/null 2>&1; then
		fetch_cmd="wget -qO $binary \"$url\""
	else
		printf "installer: neither curl nor wget found; please install one of them.\n" >&2
		exit 1
	fi

	printf "installer: downloading from %s...\n" "$url"
	eval "$fetch_cmd"

	chmod +x $binary

	if [ -d "$HOME/.local/bin" ]; then
		printf "installer: installing %s to \$HOME/.local/bin\n" "$binary"
		mv $binary "$HOME/.local/bin"
	elif [ -d "$HOME/bin" ]; then
		printf "installer: installing %s to \$HOME/bin" "$binary"
		mv $binary "$HOME/bin"
	else
		printf "installer: neither \$HOME/.local/bin nor \$HOME/bin found; leaving binary in current directory.\n" >&2
	fi

	printf "installer: %s installed successfully.\n" "$binary"
}

main
do-dyndns --version
