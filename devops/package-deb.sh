#!/bin/sh
set -eu

version=${PACKAGE_VERSION:-}
case "$version" in
    ''|[!0-9]*|*[!0-9A-Za-z.+:~-]*)
        echo "PACKAGE_VERSION must be a Debian version starting with a digit" >&2
        exit 1
        ;;
esac
dpkg --validate-version "$version"

stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
trap 'exit 1' HUP INT TERM
mkdir -p "$stage/DEBIAN" "$stage/usr/bin" "$stage/usr/share/applications"
install -m 755 dist/Moderator-Launcher "$stage/usr/bin/moderator-launcher"

cat > "$stage/DEBIAN/control" <<EOF
Package: moderator-launcher
Version: $version
Architecture: amd64
Maintainer: devalv
Section: utils
Priority: optional
Depends: libc6 (>= 2.36), libgl1, libegl1, libx11-6, libxcursor1, libxrandr2, libxi6, libxinerama1, libxxf86vm1, libwayland-client0, libwayland-cursor0, libwayland-egl1, libxkbcommon0
Description: GLINT Moderator Launcher for Astra Linux 1.8
 Prepares Space Client settings for moderator mode and restores the original INI.
EOF

cat > "$stage/usr/share/applications/moderator-launcher.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Name=Moderator Launcher
Exec=moderator-launcher
Terminal=false
Categories=Utility;
EOF

chmod 755 "$stage" "$stage/DEBIAN"
chmod 644 "$stage/DEBIAN/control" "$stage/usr/share/applications/moderator-launcher.desktop"
dpkg-deb --root-owner-group --build "$stage" "/tmp/moderator-launcher_${version}_amd64.deb"
