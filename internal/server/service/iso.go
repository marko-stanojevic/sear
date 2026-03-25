package service

import (
	"context"
	"fmt"

	"github.com/marko-stanojevic/kompakt/internal/common"
	"github.com/marko-stanojevic/kompakt/internal/iso"
)

// StartISOBuild creates a new ISO build job and starts it in the background.
// The caller is responsible for resolving secretValue from secretName.
// platform must be "linux" (default) or "winpe".
func (m *Manager) StartISOBuild(serverURL, secretName, secretValue, customName, platform, extraInstructions string, tlsSkipVerify bool) (*iso.Build, error) {
	if platform == "" {
		platform = "linux"
	}

	var agentBin string
	var err error
	switch platform {
	case "winpe":
		agentBin, err = iso.FindWindowsAgentBinary()
	default:
		agentBin, err = iso.FindAgentBinary()
	}
	if err != nil {
		return nil, fmt.Errorf("finding agent binary: %w", err)
	}

	buildID := common.NewID()
	build := m.ISOBuilds.Create(buildID, secretName, serverURL, customName, platform)

	req := iso.BuildRequest{
		ID:                          buildID,
		CustomName:                  customName,
		Platform:                    platform,
		ServerURL:                   serverURL,
		SecretName:                  secretName,
		SecretValue:                 secretValue,
		TLSSkipVerify:               tlsSkipVerify,
		AgentBinaryPath:             agentBin,
		OutputDir:                   m.ISOOutputDir,
		ExtraDockerfileInstructions: extraInstructions,
	}

	go func() {
		buildCtx, cancel := context.WithTimeout(context.Background(), iso.BuildTimeout)
		defer cancel()
		iso.RunBuild(buildCtx, m.ISOBuilds, build, req)
	}()

	return build, nil
}
