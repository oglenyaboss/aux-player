package tui

import (
	"fmt"
	"os"
	"testing"
)

// TestMain points the user cache at a scratch directory: the library store
// deletes other accounts' data under it, and the art cache writes into it.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "aux-tui-cache-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// os.UserCacheDir reads XDG_CACHE_HOME on Linux, HOME on macOS and
	// LocalAppData on Windows.
	for _, key := range []string{"XDG_CACHE_HOME", "HOME", "LocalAppData"} {
		if err := os.Setenv(key, dir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
