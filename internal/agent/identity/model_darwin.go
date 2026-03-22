//go:build darwin
// +build darwin

package identity

import (
	"log/slog"
	"os"

	"howett.net/plist"
)

// getModel returns the marketing name for the Mac model, if available.
func getModel() string {
	model := firstNonEmpty(runAndTrim("sysctl", "-n", "hw.model"))
	if model == "" {
		slog.Debug("getModel: sysctl hw.model returned empty")
		return "unknown"
	}
	plistPath := "/System/Library/PrivateFrameworks/ServerInformation.framework/SIMachineAttributes.plist"
	f, err := os.Open(plistPath)
	if err != nil {
		slog.Debug("getModel: could not open plist", "path", plistPath, "error", err)
		return model // fallback to identifier
	}
	defer f.Close()
	var data map[string]interface{}
	decoder := plist.NewDecoder(f)
	if err := decoder.Decode(&data); err != nil {
		slog.Debug("getModel: plist decode failed", "error", err)
		return model
	}
	entry, ok := data[model].(map[string]interface{})
	if !ok {
		slog.Debug("getModel: model entry not found in plist", "model", model)
		return model
	}
	loc, ok := entry["_LOCALIZABLE_"].(map[string]interface{})
	if !ok {
		slog.Debug("getModel: _LOCALIZABLE_ not found in plist entry", "model", model)
		return model
	}
	marketing, ok := loc["marketingModel"].(string)
	if ok && marketing != "" {
		slog.Debug("getModel: found marketing model", "marketingModel", marketing)
		return marketing
	}
	slog.Debug("getModel: marketingModel not found or empty", "model", model)
	return model
}
