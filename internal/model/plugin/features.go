package plugin

// FeatureCacheVars is the feature a manifest needs to hold cache_vars in a
// grant: the engine points each name a grant gives at a folder
// of its own choosing. An engine without it refuses the plugin at open.
const FeatureCacheVars = "cache-vars"

// CacheVars is the cache_vars of the grant x holds, whichever its form; nil
// for a grant that gives none, or a Program that holds no form.
func (x Program) CacheVars() []string {
	switch {
	case x.Found != nil:
		return x.Found.CacheVars
	case x.At != nil:
		return x.At.CacheVars
	case x.Script != nil:
		return x.Script.CacheVars
	}
	return nil
}
