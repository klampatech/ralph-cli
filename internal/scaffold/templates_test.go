package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBundledFilesPresent ensures all 6 expected templates are embedded
// and the embedded byte content matches the source files on disk byte-for-byte.
// This guards against accidental drift between the source files in
// /tmp/ralph-playbook/files/ and the embedded copies.
func TestBundledFilesPresent(t *testing.T) {
	sourceDir := "/tmp/ralph-playbook/files"
	for _, bf := range BundledFiles {
		t.Run(bf.Dest, func(t *testing.T) {
			embedded, err := Read(bf.FSPath)
			if err != nil {
				t.Fatalf("Read(%q) error: %v", bf.FSPath, err)
			}
			if len(embedded) == 0 {
				t.Fatalf("embedded file %q is empty", bf.FSPath)
			}

			// Compare against the source on disk if it exists.
			sourcePath := filepath.Join(sourceDir, bf.Dest)
			if _, err := os.Stat(sourcePath); err == nil {
				disk, err := os.ReadFile(sourcePath)
				if err != nil {
					t.Fatalf("read source %q: %v", sourcePath, err)
				}
				if string(disk) != string(embedded) {
					t.Fatalf("embedded copy of %q differs from source %q; re-copy", bf.FSPath, sourcePath)
				}
			}
		})
	}
}

// TestBundledFilesCount guards against accidentally adding or removing
// a template without updating SPEC §5.
func TestBundledFilesCount(t *testing.T) {
	if got, want := len(BundledFiles), 6; got != want {
		t.Fatalf("BundledFiles has %d entries, want %d (SPEC §5)", got, want)
	}
}

// TestBundledFilesChmod ensures loop.sh is executable and prompts are not.
func TestBundledFilesChmod(t *testing.T) {
	// Spot-check: loop.sh is the only executable.
	execCount := 0
	for _, bf := range BundledFiles {
		if bf.Mode&0o111 != 0 {
			execCount++
			if bf.Dest != "loop.sh" {
				t.Errorf("file %q is unexpectedly executable (mode %s)", bf.Dest, bf.Mode)
			}
		}
	}
	if execCount != 1 {
		t.Errorf("expected exactly 1 executable file, got %d", execCount)
	}
}

// TestPROMPTBuildHasTagPolicy guards against the v0.1.1 regression where
// the build prompt told Claude to tag on every clean test pass (issue #3).
// v0.1.2's rule 9999999 must mention "every 5" or similar throttle, AND
// must NOT contain the v0.1.1 "no build or test errors" trigger.
func TestPROMPTBuildHasTagPolicy(t *testing.T) {
	data, err := Read("templates/PROMPT_build.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "9999999.") {
		t.Error("PROMPT_build.md missing the 9999999. tag-policy rule")
	}
	// Must mention the "every 5th" throttle somewhere in the rule.
	if !strings.Contains(s, "every 5") {
		t.Errorf("PROMPT_build.md tag policy should throttle to every 5th iter; got:\n%s", s)
	}
	// Must NOT contain the v0.1.1 trigger phrase that produced spurious tags.
	if strings.Contains(s, "As soon as there are no build or test errors") {
		t.Error("PROMPT_build.md still has the v0.1.1 spurious-tag trigger phrase")
	}
	// Must NOT contain the v0.1.1 catch-all "create a git tag" without context.
	if strings.Contains(s, "create a git tag.") {
		t.Error("PROMPT_build.md still has the bare 'create a git tag.' rule")
	}
}

// TestPROMPTBuildMandatesPlanUpdate guards against the v0.1.1 regression
// where the loop never prompted Claude to update IMPLEMENTATION_PLAN.md
// after a commit (issue #5). v0.1.2's rule 999999999 must be marked
// MANDATORY and must require flipping `- [ ]` to `- [x]`.
func TestPROMPTBuildMandatesPlanUpdate(t *testing.T) {
	data, err := Read("templates/PROMPT_build.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "999999999.") {
		t.Error("PROMPT_build.md missing the 999999999. plan-update rule")
	}
	if !strings.Contains(s, "MANDATORY") {
		t.Error("PROMPT_build.md plan-update rule should be marked MANDATORY")
	}
	if !strings.Contains(s, "[x]") {
		t.Error("PROMPT_build.md plan-update rule should mention flipping to [x]")
	}
}

// TestPROMPTBuildDoesNotReferencePrdJson guards against the v0.1.2 prompt
// regression where rule 9999999 referenced `prd.json`, a file the v0.1.x
// scaffold does not create (SPEC §13 defers the schema to v0.2). v0.1.3
// removed the prd.json reference and reworded the rule to use
// `IMPLEMENTATION_PLAN.md` (issue #16).
func TestPROMPTBuildDoesNotReferencePrdJson(t *testing.T) {
	data, err := Read("templates/PROMPT_build.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	// Rule 9999999 must still exist (regression guard against accidentally
	// deleting the throttle policy altogether).
	if !strings.Contains(s, "9999999.") {
		t.Error("PROMPT_build.md missing the 9999999. tag-policy rule")
	}
	// The new rule (starting at the 9999999. prefix) should reference
	// IMPLEMENTATION_PLAN.md, not prd.json. We isolate the rule body by
	// splitting on rule-number prefixes to avoid matching the "(Note: ...
	// referenced `prd.json`)" deprecation comment that the v0.1.3 fix
	// added for human readers.
	var ruleBody string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "9999999.") {
			// Collect this line + subsequent non-rule-number lines until
			// the next rule-number prefix.
			lines := []string{line}
			rest := strings.Split(s, line+"\n")
			if len(rest) < 2 {
				break
			}
			for _, l := range strings.Split(rest[1], "\n") {
				t := strings.TrimSpace(l)
				if t == "" {
					continue
				}
				// Stop at the next rule (which starts with digits followed
				// by a dot).
				if len(t) > 1 && t[0] >= '0' && t[0] <= '9' {
					break
				}
				lines = append(lines, l)
			}
			ruleBody = strings.Join(lines, "\n")
			break
		}
	}
	if ruleBody == "" {
		t.Fatal("could not locate rule 9999999 body in PROMPT_build.md")
	}
	if strings.Contains(ruleBody, "prd.json") {
		t.Errorf("PROMPT_build.md rule 9999999 still references prd.json; v0.1.x scaffold does not create it. Rule body:\n%s", ruleBody)
	}
	if !strings.Contains(ruleBody, "IMPLEMENTATION_PLAN.md") {
		t.Errorf("PROMPT_build.md rule 9999999 should reference IMPLEMENTATION_PLAN.md. Rule body:\n%s", ruleBody)
	}
}
