package fs

import "github.com/birdtable/gragas/internal/pluginbundle"

// LockEntry is pluginbundle.LockEntry: the plugin lock is pluginbundle's,
// which the core's plan of an add reads and writes too.
type LockEntry = pluginbundle.LockEntry

const (
	// PluginLockVersion is the newest version of the plugin lock this gragas
	// reads and the one it writes.
	PluginLockVersion = pluginbundle.PluginLockVersion
	// PluginLockPath is the lock's path in a rulebook's root.
	PluginLockPath = pluginbundle.PluginLockPath
)

// DecodePluginLock is pluginbundle.DecodePluginLock.
func DecodePluginLock(b []byte) ([]LockEntry, error) { return pluginbundle.DecodePluginLock(b) }

// EncodePluginLock is pluginbundle.EncodePluginLock.
func EncodePluginLock(entries []LockEntry) []byte { return pluginbundle.EncodePluginLock(entries) }
