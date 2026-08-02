package runtime

import (
	"fmt"
	"strings"
)

type Backend string

const (
	BackendAuto   Backend = "auto"
	BackendHerdr  Backend = "herdr"
	BackendTmux   Backend = "tmux"
	BackendDirect Backend = "direct"
)

func ParseBackend(value string) (Backend, error) {
	switch Backend(strings.ToLower(strings.TrimSpace(value))) {
	case BackendAuto:
		return BackendAuto, nil
	case BackendHerdr:
		return BackendHerdr, nil
	case BackendTmux:
		return BackendTmux, nil
	case BackendDirect:
		return BackendDirect, nil
	default:
		return "", fmt.Errorf(
			"unsupported backend %q: expected auto, herdr, tmux, or direct",
			value,
		)
	}
}

type Probe struct {
	HerdrBinary  bool `json:"herdr_binary"`
	HerdrManaged bool `json:"herdr_managed"`
	TmuxBinary   bool `json:"tmux_binary"`
}

func (p Probe) HerdrReady() bool {
	return p.HerdrBinary && p.HerdrManaged
}

func Select(requested Backend, probe Probe) (Backend, error) {
	switch requested {
	case BackendAuto:
		switch {
		case probe.HerdrReady():
			return BackendHerdr, nil
		case probe.TmuxBinary:
			return BackendTmux, nil
		default:
			return BackendDirect, nil
		}
	case BackendHerdr:
		if !probe.HerdrBinary {
			return "", fmt.Errorf("herdr backend was requested but the herdr executable was not found")
		}
		return BackendHerdr, nil
	case BackendTmux:
		if !probe.TmuxBinary {
			return "", fmt.Errorf("tmux backend was requested but the tmux executable was not found")
		}
		return BackendTmux, nil
	case BackendDirect:
		return BackendDirect, nil
	default:
		return "", fmt.Errorf("unknown backend %q", requested)
	}
}
