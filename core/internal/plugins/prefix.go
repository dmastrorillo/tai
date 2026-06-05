package plugins

// PluginTagPrefix returns the GitHub-release tag prefix the host
// should use when resolving "latest version" for an installed
// plugin. Pinned by the `release-cycle` capability: first-party
// plugins shipped from this monorepo use tags
// `plugins/<name>/vX.Y.Z`; third-party plugins shipped from their
// own repos may use any tag convention (typically bare `vX.Y.Z`).
// The fetcher (Install/Update without --version) and the update-
// banner background poll both call this per installed plugin.
//
//   - "plugins/<name>/" when src matches the built-in registry
//     entry for `name` (host AND repo equal) — i.e. the plugin is
//     first-party and follows the prefixed-tag convention.
//   - ""                 otherwise — third-party plugins ship from
//     repos that we don't control and have no enforced prefix.
//     LatestPrefixedTag with an empty prefix matches every tag.
//
// Pure (no I/O), cheap (single map lookup).
func PluginTagPrefix(name string, src Source) string {
	registered, ok := Lookup(name)
	if !ok {
		return ""
	}
	if registered.Host == src.Host && registered.Repo == src.Repo {
		return "plugins/" + name + "/"
	}
	return ""
}
