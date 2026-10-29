package sitebuild

import "vozko/domain/site"

func SnippetSection(snippet site.DesignSnippet) SectionInfo {
	return SectionInfo{
		Key:    snippet.Key,
		Title:  SectionTitle{PT: snippet.Name, EN: snippet.Name, ES: snippet.Name, DE: snippet.Name},
		Stacks: snippetStacks(snippet.Stack),
		Kind:   SectionSnippet,
		Group:  GroupSaved,
	}
}
