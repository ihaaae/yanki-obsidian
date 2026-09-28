/**
 * The settings schema for Yanki, its defaults, and the namespace helpers.
 *
 * This module is deliberately free of Obsidian imports: the plugin renders and
 * persists these settings, while the command line interface reads the same
 * settings file with no Obsidian runtime available.
 */

export type YankiPluginSettings = {
	ankiConnect: {
		host: string
		key: string | undefined
		port: number
	}
	folders: string[]
	ignoreFolderNotes: boolean
	manageFilenames: {
		autoRenameDebounceIntervalMs: number // Not exposed in settings
		autoRenameTrigger: 'before-sync' | 'file-changed' | 'off'
		maxLength: number
		mode: 'prompt' | 'response'
	}
	namespace: string
	showAdvancedSettings: boolean
	stats: {
		sync: {
			auto: number
			duration: number
			errors: number
			invalid: number
			latestSyncTime: number | undefined
			manual: number
			notes: {
				ankiUnreachable: number
				created: number
				deleted: number
				matched: number
				unchanged: number
				updated: number
			}
		}
	}
	sync: {
		autoSyncDebounceIntervalMs: number // Not exposed in settings
		autoSyncEnabled: boolean
		mediaMode: 'all' | 'local' | 'off' | 'remote'
		pushToAnkiWeb: boolean
	}
	verboseNotices: boolean
}

/**
 * Default settings, with the namespace derived from the given Obsidian vault
 * ID. Pass `undefined` when no vault ID is known, which falls back to the
 * namespace used by the stand-alone Yanki CLI tool.
 *
 * Warning: changing the static components of the namespace string can result in
 * data loss, since it's how Yanki recognizes the notes it manages.
 */
export function getYankiPluginDefaultSettings(vaultId: string | undefined): YankiPluginSettings {
	return {
		ankiConnect: {
			host: 'http://localhost',
			key: undefined,
			port: 8765,
		},
		folders: [],
		ignoreFolderNotes: true,
		manageFilenames: {
			// Obsidian already debounces this!
			autoRenameDebounceIntervalMs: 300,
			autoRenameTrigger: 'off',
			maxLength: 60,
			mode: 'prompt',
		},
		// Defaults to the vault ID the first time Yanki is run on a vault, but it may NOT be the actual current vault ID, e.g. when syncing is involved
		// Using vault ID instead of name is more robust to vault renaming
		// But why is the vault ID API private?
		// https://forum.obsidian.md/t/is-there-any-way-to-derive-the-vault-id-from-the-vault-directory/5573/4
		namespace:
			vaultId === undefined ? 'Yanki' : `Yanki Obsidian - Vault ID ${sanitizeNamespace(vaultId)}`,
		showAdvancedSettings: false,
		stats: {
			sync: {
				auto: 0,
				duration: 0,
				errors: 0,
				invalid: 0,
				latestSyncTime: undefined,
				manual: 0,
				notes: {
					ankiUnreachable: 0,
					created: 0,
					deleted: 0,
					matched: 0,
					unchanged: 0,
					updated: 0,
				},
			},
		},
		sync: {
			autoSyncDebounceIntervalMs: 4000,
			autoSyncEnabled: false,
			mediaMode: 'local',
			pushToAnkiWeb: true,
		},
		verboseNotices: false,
	}
}

/**
 * Checks whether a namespace string is valid and already sanitized.
 */
export function validateNamespace(namespace: string): boolean {
	const sanitizedNamespace = sanitizeNamespace(namespace)
	return sanitizedNamespace.length > 0 && namespace === sanitizedNamespace
}

/**
 * Strips invalid characters (`*`, `:`) from a namespace string.
 */
export function sanitizeNamespace(namespace: string): string {
	// Additional sanitization also happens inside Yanki
	// Stuck with es2020?
	return namespace.replace(/[*:]/gv, '').trim()
}
