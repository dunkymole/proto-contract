package contract

import (
	"fmt"
	"regexp"
)

type ReleaseManifest struct {
	Format   int       `json:"format"`
	Releases []Release `json:"releases"`
}

type Release struct {
	API     string `json:"api"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

var releaseDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func ReadReleaseManifest(path string) (*ReleaseManifest, error) {
	b, err := readBoundedFile(path)
	if err != nil {
		return nil, err
	}
	var manifest ReleaseManifest
	if err := decodeStrictJSON(b, &manifest); err != nil {
		return nil, err
	}
	if err := ValidateReleaseManifest(&manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func ValidateReleaseManifest(manifest *ReleaseManifest) error {
	if manifest == nil || manifest.Format != 1 || manifest.Releases == nil {
		return fmt.Errorf("release manifest must use format 1 and contain a releases array")
	}
	seen := map[string]bool{}
	last := map[string][3]uint64{}
	for i, release := range manifest.Releases {
		if !apiPattern.MatchString(release.API) {
			return fmt.Errorf("invalid API identifier %q in release manifest", release.API)
		}
		parts, err := versionParts(release.Version)
		if err != nil {
			return fmt.Errorf("invalid release version: %w", err)
		}
		if !releaseDigestPattern.MatchString(release.Digest) {
			return fmt.Errorf("invalid digest for %s %s", release.API, release.Version)
		}
		key := release.API + "\x00" + release.Version
		if seen[key] {
			return fmt.Errorf("duplicate release assignment for %s %s", release.API, release.Version)
		}
		seen[key] = true
		if prior, ok := last[release.API]; ok && compareVersionParts(parts, prior) <= 0 {
			return fmt.Errorf("release history for %s is not strictly increasing at entry %d", release.API, i)
		}
		last[release.API] = parts
	}
	return nil
}

// CheckReleaseTransition verifies immutable release identity and the minimum
// structural bump. The proposed manifest must preserve the trusted prefix.
func CheckReleaseTransition(base, proposed *Snapshot, baseHistory, proposedHistory *ReleaseManifest) error {
	if err := Validate(base); err != nil {
		return fmt.Errorf("invalid base lock: %w", err)
	}
	if err := Validate(proposed); err != nil {
		return fmt.Errorf("invalid proposed lock: %w", err)
	}
	if err := ValidateReleaseManifest(baseHistory); err != nil {
		return fmt.Errorf("invalid trusted release history: %w", err)
	}
	if err := ValidateReleaseManifest(proposedHistory); err != nil {
		return fmt.Errorf("invalid proposed release history: %w", err)
	}
	if base.API != proposed.API {
		return fmt.Errorf("API identifier changed from %q to %q", base.API, proposed.API)
	}
	if base.Service.Name != proposed.Service.Name {
		return fmt.Errorf("service identity changed from %q to %q", base.Service.Name, proposed.Service.Name)
	}
	if len(proposedHistory.Releases) < len(baseHistory.Releases) {
		return fmt.Errorf("release history is not append-only: entries were removed")
	}
	for i, entry := range baseHistory.Releases {
		if proposedHistory.Releases[i] != entry {
			return fmt.Errorf("release history is not append-only: trusted entry %d was changed or reordered", i)
		}
	}
	bootstrap := len(baseHistory.Releases) == 0
	if !bootstrap {
		latest, ok := latestAssignment(baseHistory, base.API)
		if !ok || latest.Version != base.Version || latest.Digest != base.Digest {
			return fmt.Errorf("stale trusted baseline: lock %s %s is not the latest trusted assignment for %s", base.API, base.Version, base.API)
		}
	}
	baseParts, _ := versionParts(base.Version)
	currentParts, _ := versionParts(proposed.Version)
	order := compareVersionParts(currentParts, baseParts)
	if order < 0 {
		return fmt.Errorf("version regression from %s to %s", base.Version, proposed.Version)
	}
	if order == 0 {
		if base.Digest != proposed.Digest {
			return fmt.Errorf("schema fork: %s %s is already assigned digest %s, proposed %s", base.API, base.Version, base.Digest, proposed.Digest)
		}
		if err := requireAssignment(proposedHistory, proposed); err != nil {
			return err
		}
		if bootstrap {
			if len(proposedHistory.Releases) != 1 || proposedHistory.Releases[0] != releaseFor(base) {
				return fmt.Errorf("bootstrap history must contain only the trusted base lock assignment")
			}
		} else if len(proposedHistory.Releases) != len(baseHistory.Releases) {
			return fmt.Errorf("release history adds an assignment without a new version")
		}
		return nil
	}
	minimum, _ := Compare(base, proposed)
	if !versionMeetsBump(baseParts, currentParts, minimum) {
		return fmt.Errorf("version %s does not meet structural %s bump from %s", proposed.Version, minimum, base.Version)
	}
	if bootstrap {
		if len(proposedHistory.Releases) != 2 || proposedHistory.Releases[0] != releaseFor(base) || proposedHistory.Releases[1] != releaseFor(proposed) {
			return fmt.Errorf("bootstrap history must seed the trusted base assignment and append exactly the proposed release")
		}
	} else {
		if len(proposedHistory.Releases) != len(baseHistory.Releases)+1 {
			return fmt.Errorf("new release must append exactly one assignment")
		}
		if proposedHistory.Releases[len(proposedHistory.Releases)-1] != releaseFor(proposed) {
			return fmt.Errorf("new release must append the proposed API/version/digest assignment")
		}
	}
	if err := requireAssignment(proposedHistory, proposed); err != nil {
		return err
	}
	return nil
}

func releaseFor(snapshot *Snapshot) Release {
	return Release{API: snapshot.API, Version: snapshot.Version, Digest: snapshot.Digest}
}

func latestAssignment(history *ReleaseManifest, api string) (Release, bool) {
	var latest Release
	var latestParts [3]uint64
	found := false
	for _, release := range history.Releases {
		if release.API != api {
			continue
		}
		parts, _ := versionParts(release.Version)
		if !found || compareVersionParts(parts, latestParts) > 0 {
			latest, latestParts, found = release, parts, true
		}
	}
	return latest, found
}

func requireAssignment(history *ReleaseManifest, snapshot *Snapshot) error {
	for _, release := range history.Releases {
		if release.API == snapshot.API && release.Version == snapshot.Version {
			if release.Digest != snapshot.Digest {
				return fmt.Errorf("schema fork: %s %s is assigned %s, proposed %s", snapshot.API, snapshot.Version, release.Digest, snapshot.Digest)
			}
			return nil
		}
	}
	return fmt.Errorf("release history has no assignment for %s %s", snapshot.API, snapshot.Version)
}

func compareVersionParts(a, b [3]uint64) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func versionMeetsBump(base, current [3]uint64, minimum Bump) bool {
	switch minimum {
	case Major:
		return current[0] > base[0]
	case Minor:
		return current[0] > base[0] || current[0] == base[0] && current[1] > base[1]
	default:
		return true
	}
}
