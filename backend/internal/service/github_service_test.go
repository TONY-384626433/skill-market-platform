package service

import (
	"testing"

	"github.com/jjbank/skill-market/internal/model"
)

func TestParseSkillDocument(t *testing.T) {
	metadata, body := parseSkillDocument(`---
name: browser-operator
description: Automates browser workflows.
license: MIT
compatibility: Requires Node.js
---
# Browser Operator

Use this skill for repeatable browser tasks.`)
	if metadata.Name != "browser-operator" || metadata.Description != "Automates browser workflows." {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	if body == "" {
		t.Fatal("expected markdown body")
	}
}

func TestCompatibilityForHost(t *testing.T) {
	githubService := &GitHubService{host: model.GitHubHostProfile{
		OS:       "windows",
		Runtimes: map[string]bool{"node": true, "python": false, "docker": false},
	}}

	installable := githubService.compatibilityFor("Run with npm install and npx tool", "Requires Node.js")
	if installable.Status != "installable" {
		t.Fatalf("expected installable, got %#v", installable)
	}

	needsSetup := githubService.compatibilityFor("Requires Python 3 and pip install package", "")
	if needsSetup.Status != "needs_setup" {
		t.Fatalf("expected needs_setup, got %#v", needsSetup)
	}

	incompatible := githubService.compatibilityFor("Desktop automation", "macOS only")
	if incompatible.Status != "incompatible" {
		t.Fatalf("expected incompatible, got %#v", incompatible)
	}
}

func TestGitHubSkillClassification(t *testing.T) {
	cases := map[string]string{
		"security audit and vulnerability scanning": "安全合规",
		"browser automation with playwright":        "自动化",
		"financial investment research":             "金融业务",
		"presentation and document writing":         "内容创作",
		"unclassified assistant":                    "通用智能",
	}
	for input, expected := range cases {
		if actual := categorizeGitHubSkill(input); actual != expected {
			t.Errorf("categorizeGitHubSkill(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestPlainSearchTermsRemovesQualifiers(t *testing.T) {
	if actual := plainSearchTerms(`browser repo:private "automation"`); actual != "browser repo private automation" {
		t.Fatalf("unexpected sanitized query: %q", actual)
	}
}

func TestArchivePathSelection(t *testing.T) {
	if !archivePathSelected("skills/browser/SKILL.md", "skills/browser") {
		t.Fatal("expected nested SKILL.md to be selected")
	}
	if archivePathSelected("skills/other/SKILL.md", "skills/browser") {
		t.Fatal("must not include sibling skill")
	}
	if !archivePathSelected("scripts/run.py", "") {
		t.Fatal("expected root scripts to be selected")
	}
}

func TestNormalizeSkillLocator(t *testing.T) {
	repository, ref, skillPath, err := normalizeSkillLocator(" owner/repo ", "main", "skills/browser/SKILL.md")
	if err != nil || repository != "owner/repo" || ref != "main" || skillPath != "skills/browser/SKILL.md" {
		t.Fatalf("unexpected normalized locator: %q %q %q %v", repository, ref, skillPath, err)
	}
	invalid := [][3]string{
		{"invalid", "main", "SKILL.md"},
		{"owner/repo", "../main", "SKILL.md"},
		{"owner/repo", "main", "../SKILL.md"},
		{"owner/repo", "main", "README.md"},
	}
	for _, input := range invalid {
		if _, _, _, err := normalizeSkillLocator(input[0], input[1], input[2]); err == nil {
			t.Fatalf("expected locator to be rejected: %#v", input)
		}
	}
}
