// Package assets embeds the AUX icon and .desktop file and provides helpers
// that make sure the desktop can find them, writing fallbacks into the user's
// data directory only when no installer already provided them.
package assets

import (
	_ "embed"
)

// AppID is the desktop identity AUX is known by: the basename of its
// .desktop file, its icon name, and the MPRIS DesktopEntry property. The spec
// resolves that property to "<AppID>.desktop", so the three have to be one
// string or a desktop that finds the player over MPRIS cannot find its entry
// or icon. The install script and the Flatpak manifest use the same id.
const AppID = "io.github.oglenyaboss.aux"

//go:embed aux-icon.svg
var Icon []byte

//go:embed io.github.oglenyaboss.aux.desktop
var DesktopEntry []byte
