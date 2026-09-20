package migration

import (
	"errors"
	"io"
	"os"
)

// LoadLegacyCenterFile reads one explicitly selected regular legacy catalog.
// It follows no links, writes nothing, and never includes the private path in
// returned errors.
func LoadLegacyCenterFile(path string) (LegacyCenterCompatibility, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return LegacyCenterCompatibility{}, errors.New("legacy Center catalog could not be inspected")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > MaxInputBytes {
		return LegacyCenterCompatibility{}, errors.New("legacy Center catalog must be one bounded regular file, not a link")
	}
	file, err := os.Open(path)
	if err != nil {
		return LegacyCenterCompatibility{}, errors.New("legacy Center catalog could not be opened")
	}
	defer file.Close()
	input, err := io.ReadAll(io.LimitReader(file, MaxInputBytes+1))
	if err != nil || len(input) == 0 || len(input) > MaxInputBytes {
		return LegacyCenterCompatibility{}, errors.New("legacy Center catalog could not be read safely")
	}
	return LoadLegacyCenterCatalog(input)
}
