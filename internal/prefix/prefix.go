// Package prefix resolves Node installation prefixes (the active npm prefix
// in v1; version-manager discovery is added on top of this).
package prefix

import (
	"context"
	"os/exec"
	"strings"
)

// Active returns the absolute path of the active npm prefix — the one plain
// `npm` commands would use.
func Active(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "npm", "config", "get", "prefix").Output()
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return "", exec.ErrNotFound
	}
	return p, nil
}
