package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"unicode/utf8"

	"skillshare/internal/instructions"
	syncpkg "skillshare/internal/sync"
)

// Memory guidance is a marked block (see Instructions) added to the
// instruction files targets already read. Connecting never changes the
// config, assignments, or links: the block goes into the file the target
// reads, and everything outside the block is kept. Status only reports what
// the files and links show; it never claims an agent read the notes.

// ErrGuidanceStale reports that a reviewed plan no longer matches the files
// or the config it was made from.
var ErrGuidanceStale = errors.New("instruction files changed since the preview; review the changes again")

// GuidanceSource is one file in a target's read chain.
type GuidanceSource struct {
	Read   string // the path whose content the target gets
	Write  string // the real file to change for it; empty means Read, or its destination when Read is a link
	Shared string // the shared instruction file it is, if any
	Live   bool   // false when the shared file is attached but not synced
}

// GuidanceReader is one target and the chain of files it reads.
type GuidanceReader struct {
	Name     string
	Sources  []GuidanceSource
	Dest     int // the source that receives a new block
	MaxChars int
}

// GuidanceInput is what the caller resolved from its config: the memory
// folder, the scope, and every target's read chain.
type GuidanceInput struct {
	Root        string // the memory folder
	ProjectRoot string // empty in global scope
	Readers     []GuidanceReader
	Config      any // hashed into the plan token, so a config change makes a plan stale
}

type GuidanceTarget struct {
	Name   string `json:"name"`
	State  string `json:"state"`            // unconfigured, configured, outdated or broken
	File   string `json:"file,omitempty"`   // where the block is, or would be written
	Detail string `json:"detail,omitempty"` // why broken: modified, malformed, mixed_modes, not_synced, unreadable or unsupported
	Mode   string `json:"mode,omitempty"`   // passive or active, when a block is found
}

type GuidanceChange struct {
	Path    string   `json:"path"`
	Before  string   `json:"before"`
	After   string   `json:"after"`
	Targets []string `json:"targets"`
	Created bool     `json:"created"`
	Shared  string   `json:"-"` // the shared instruction file Path is, if any
}

type GuidanceSkip struct {
	Target string `json:"target"`
	Reason string `json:"reason"`
}

type GuidanceWarning struct {
	Code    string   `json:"code"` // also_read_by or over_limit
	Path    string   `json:"path"`
	Targets []string `json:"targets,omitempty"`
	Target  string   `json:"target,omitempty"`
	Limit   int      `json:"limit,omitempty"`
	Chars   int      `json:"chars,omitempty"`
}

type GuidancePlan struct {
	Token    string            `json:"token"`
	Changes  []GuidanceChange  `json:"changes"`
	Skipped  []GuidanceSkip    `json:"skipped"`
	Warnings []GuidanceWarning `json:"warnings"`
}

// GuidanceFailure is one file that could not be written or synced. Err is
// ErrGuidanceStale when the file no longer matches its reviewed content.
type GuidanceFailure struct {
	Path string
	Err  error
}

// GuidanceResult lists the files ApplyGuidance wrote and what failed.
type GuidanceResult struct {
	Applied  []string
	Failures []GuidanceFailure
}

// guidanceSite is a target's chain and where a new block would go.
type guidanceSite struct {
	GuidanceTarget
	sources  []GuidanceSource
	dest     int    // the source that receives a new block
	blocks   int    // live sources holding an intact block
	shared   string // shared file of File, for syncing copies
	maxChars int
}

func (in GuidanceInput) scope() string {
	if in.ProjectRoot != "" {
		return ScopeProject
	}
	return ScopeGlobal
}

func (in GuidanceInput) instructions(mode string) string {
	return Instructions(in.Root, in.ProjectRoot, mode)
}

// realFile returns the file a write to path changes: a link's destination.
func realFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	return filepath.EvalSymlinks(path)
}

// GuidanceStatus reports the state of every reader's chain.
func GuidanceStatus(in GuidanceInput) []GuidanceTarget {
	sites := guidanceSites(in)
	out := make([]GuidanceTarget, 0, len(sites))
	for _, site := range sites {
		out = append(out, site.GuidanceTarget)
	}
	return out
}

func guidanceSites(in GuidanceInput) []guidanceSite {
	out := make([]guidanceSite, 0, len(in.Readers))
	for _, r := range in.Readers {
		site := guidanceSite{GuidanceTarget: GuidanceTarget{Name: r.Name}, sources: slices.Clone(r.Sources), dest: r.Dest, maxChars: r.MaxChars}
		for i, src := range site.sources {
			if src.Write != "" {
				continue
			}
			// The file may itself link elsewhere: change, preview and back up its real file.
			write, err := realFile(src.Read)
			if err != nil {
				site.State, site.Detail, site.File = "broken", "unreadable", src.Read
			}
			site.sources[i].Write = write
		}
		out = append(out, resolveSite(site, in))
	}
	return out
}

// resolveSite sets the state from the chain: a current block anywhere the
// target reads wins; then a block it cannot be trusted with; then an
// outdated one. Without a block, site.dest receives it. Each block is checked
// against the text of the mode it records; blocks of different modes in one
// chain contradict each other, and rewriting one file would not fix that.
func resolveSite(site guidanceSite, in GuidanceInput) guidanceSite {
	if site.State != "" {
		return site
	}
	rank := map[string]int{StateConfigured: 4, StateModified: 3, StateMalformed: 3, StateOutdated: 2}
	best, bestRank := -1, 0
	modes := map[string]bool{}
	for i, src := range site.sources {
		data, err := os.ReadFile(src.Read)
		if err != nil && !os.IsNotExist(err) {
			site.State, site.Detail, site.File = "broken", "unreadable", src.Write
			return site
		}
		// JSON previews cannot show other encodings byte for byte, so such a
		// file is never inspected or rewritten.
		if !utf8.Valid(data) {
			site.State, site.Detail, site.File = "broken", "unsupported", src.Write
			return site
		}
		mode := Mode(string(data), in.scope())
		state := Inspect(string(data), in.instructions(mode))
		if !src.Live {
			if state != StateUnconfigured {
				site.State, site.Detail, site.File, site.shared = "broken", "not_synced", src.Write, src.Shared
				return site
			}
			continue
		}
		if state != StateUnconfigured && state != StateMalformed {
			modes[mode] = true
			site.blocks++
		}
		if rank[state] > bestRank {
			best, bestRank = i, rank[state]
			site.Detail, site.Mode = state, mode
		}
	}
	if best >= 0 {
		src := site.sources[best]
		site.File, site.shared = src.Write, src.Shared
		if len(modes) > 1 {
			site.State, site.Detail, site.Mode = "broken", "mixed_modes", ""
			return site
		}
		switch site.Detail {
		case StateConfigured:
			site.State, site.Detail = "configured", ""
		case StateOutdated:
			site.State, site.Detail = "outdated", ""
		default:
			site.State = "broken"
		}
		return site
	}
	site.State = "unconfigured"
	dest := site.sources[site.dest]
	if !dest.Live {
		site.State, site.Detail = "broken", "not_synced"
	}
	site.File, site.shared = dest.Write, dest.Shared
	return site
}

// PlanGuidance builds the changes for the named targets, each in the mode
// modes gives it; a target left out keeps its block's mode, or gets passive.
// Targets reading one file share its block, so they must share a mode. The
// token hashes everything the plan depends on, so ApplyGuidance can reject a
// plan whose files or config changed after review.
func PlanGuidance(in GuidanceInput, names []string, modes map[string]string) (GuidancePlan, error) {
	sites := guidanceSites(in)
	byName := map[string]guidanceSite{}
	for _, site := range sites {
		byName[site.Name] = site
	}
	names = slices.Compact(slices.Sorted(slices.Values(names)))
	if len(names) == 0 {
		return GuidancePlan{}, errors.New("choose at least one target")
	}
	plan := GuidancePlan{Changes: []GuidanceChange{}, Skipped: []GuidanceSkip{}, Warnings: []GuidanceWarning{}}
	index := map[string]int{}
	chosen, fileModes := map[string]string{}, map[string]string{}
	for _, name := range names {
		site, ok := byName[name]
		if !ok {
			return GuidancePlan{}, errors.New("target has no instruction file: " + name)
		}
		mode, err := ParseMode(modes[name])
		if err != nil {
			return GuidancePlan{}, err
		}
		if modes[name] == "" && site.Mode != "" {
			mode = site.Mode
		}
		if other, ok := fileModes[site.File]; ok && other != mode && site.File != "" {
			return GuidancePlan{}, errors.New("targets reading " + site.File + " share its guidance and must use the same mode")
		}
		fileModes[site.File], chosen[name] = mode, mode
		switch {
		case site.State == "configured" && site.Mode == mode:
			plan.Skipped = append(plan.Skipped, GuidanceSkip{name, "configured"})
			continue
		case site.State == "broken":
			plan.Skipped = append(plan.Skipped, GuidanceSkip{name, site.Detail})
			continue
		case site.Mode != mode && site.blocks > 1:
			// Rewriting only site.File would leave the other blocks in the old mode.
			plan.Skipped = append(plan.Skipped, GuidanceSkip{name, "multiple_blocks"})
			continue
		}
		if i, ok := index[site.File]; ok {
			plan.Changes[i].Targets = append(plan.Changes[i].Targets, name)
			continue
		}
		want := in.instructions(mode)
		data, readErr := os.ReadFile(site.File)
		if readErr != nil && !os.IsNotExist(readErr) {
			plan.Skipped = append(plan.Skipped, GuidanceSkip{name, "unreadable"})
			continue
		}
		if !utf8.Valid(data) {
			plan.Skipped = append(plan.Skipped, GuidanceSkip{name, "unsupported"})
			continue
		}
		after, err := Apply(string(data), want)
		if err != nil {
			plan.Skipped = append(plan.Skipped, GuidanceSkip{name, Inspect(string(data), want)})
			continue
		}
		if after == string(data) {
			plan.Skipped = append(plan.Skipped, GuidanceSkip{name, "configured"})
			continue
		}
		index[site.File] = len(plan.Changes)
		plan.Changes = append(plan.Changes, GuidanceChange{Path: site.File, Before: string(data), After: after, Targets: []string{name}, Created: os.IsNotExist(readErr), Shared: site.shared})
	}
	for _, c := range plan.Changes {
		// Every reader of the changed file is affected, selected or not.
		var others []string
		chars := utf8.RuneCountInString(c.After)
		for _, site := range sites {
			if !slices.ContainsFunc(site.sources, func(src GuidanceSource) bool { return src.Write == c.Path }) {
				continue
			}
			if !slices.Contains(c.Targets, site.Name) {
				others = append(others, site.Name)
			}
			if site.maxChars > 0 && chars > site.maxChars {
				plan.Warnings = append(plan.Warnings, GuidanceWarning{Code: "over_limit", Path: c.Path, Target: site.Name, Limit: site.maxChars, Chars: chars})
			}
		}
		if len(others) > 0 {
			sort.Strings(others)
			plan.Warnings = append(plan.Warnings, GuidanceWarning{Code: "also_read_by", Path: c.Path, Targets: others})
		}
	}
	// Bind the routing too: a mode or assignment change can leave the
	// reviewed text equal while changing which files the block reaches.
	type route struct {
		Name, State, Detail, File, Shared string
		Sources                           [][4]string
	}
	routes := make([]route, 0, len(sites))
	for _, site := range sites {
		r := route{site.Name, site.State, site.Detail, site.File, site.shared, nil}
		for _, src := range site.sources {
			r.Sources = append(r.Sources, [4]string{src.Read, src.Write, src.Shared, strconv.FormatBool(src.Live)})
		}
		routes = append(routes, r)
	}
	data, err := json.Marshal(struct {
		Root   string
		Names  []string
		Modes  map[string]string
		Plan   GuidancePlan
		Routes []route
		Config any
	}{in.Root, names, chosen, plan, routes, in.Config})
	if err != nil {
		return GuidancePlan{}, err
	}
	sum := sha256.Sum256(data)
	plan.Token = hex.EncodeToString(sum[:])
	return plan, nil
}

// ApplyGuidance applies a reviewed plan only if recomputing it gives the same
// token; otherwise it returns ErrGuidanceStale and writes nothing. Each
// existing file is backed up before it is changed. written, when set, runs
// after each file is written, so the caller can sync its copies before the
// next file is checked; the failures it returns join the result.
func ApplyGuidance(in GuidanceInput, names []string, modes map[string]string, token string, written func(GuidanceChange) []GuidanceFailure) (GuidanceResult, error) {
	plan, err := PlanGuidance(in, names, modes)
	if err != nil {
		return GuidanceResult{}, err
	}
	if token == "" || token != plan.Token {
		return GuidanceResult{}, ErrGuidanceStale
	}
	res := GuidanceResult{Applied: []string{}, Failures: []GuidanceFailure{}}
	for _, c := range plan.Changes {
		if err := writeGuidanceChange(c); err != nil {
			res.Failures = append(res.Failures, GuidanceFailure{Path: c.Path, Err: err})
			continue
		}
		res.Applied = append(res.Applied, c.Path)
		if written != nil {
			res.Failures = append(res.Failures, written(c)...)
		}
	}
	return res, nil
}

// backupGuidanceFile is replaced in tests to change a file during its backup.
var backupGuidanceFile = syncpkg.BackupFile

// writeGuidanceChange rechecks each file as earlier writes and syncs may take
// time, and again after the backup, immediately before the write.
func writeGuidanceChange(c GuidanceChange) error {
	if err := checkGuidanceChange(c); err != nil {
		return err
	}
	if !c.Created {
		if err := backupGuidanceFile(c.Path, syncpkg.BackupReasonEdit); err != nil {
			return err
		}
		if err := checkGuidanceChange(c); err != nil {
			return err
		}
	}
	return commitGuidanceChange(c)
}

// commitGuidanceChange writes a file after its final review check. A new
// file is created exclusively, so one a competing process made is kept.
func commitGuidanceChange(c GuidanceChange) error {
	if !c.Created {
		return instructions.WriteFile(c.Path, c.After)
	}
	if err := os.MkdirAll(filepath.Dir(c.Path), 0755); err != nil {
		return err
	}
	err := createExclusive(c.Path, func(f *os.File) error {
		_, err := f.WriteString(c.After)
		return err
	})
	if os.IsExist(err) {
		return ErrGuidanceStale
	}
	return err
}

func checkGuidanceChange(c GuidanceChange) error {
	data, err := os.ReadFile(c.Path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if c.Created != os.IsNotExist(err) || string(data) != c.Before {
		return ErrGuidanceStale
	}
	return nil
}
