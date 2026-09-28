import type { ArgumentsCamelCase } from 'yargs'
import fs from 'node:fs/promises'
import { urlToHostAndPort } from 'yanki'
import type { YankiPluginSettings } from '../settings/model'
import type { CliArguments } from './options'
import type { SyncDirectory } from './vault'
import {
	getYankiPluginDefaultSettings,
	sanitizeNamespace,
	validateNamespace,
} from '../settings/model'

/**
 * A settings file may be the plugin's own `data.json` or a hand-written file,
 * and may set only the fields it cares about.
 */
type PartialYankiPluginSettings = Partial<
	Omit<YankiPluginSettings, 'ankiConnect' | 'manageFilenames' | 'stats' | 'sync'>
> & {
	ankiConnect?: Partial<YankiPluginSettings['ankiConnect']>
	manageFilenames?: Partial<YankiPluginSettings['manageFilenames']>
	sync?: Partial<YankiPluginSettings['sync']>
}

/** Where the namespace in effect came from, for reporting. */
type NamespaceSource =
	'directory name' | 'settings file' | 'the --namespace option' | 'the Obsidian vault ID'

export type ResolvedSettings = {
	/** Flashcard folders, relative to the synced directory. */
	folders: string[]
	namespace: string
	namespaceSource: NamespaceSource
	settings: YankiPluginSettings
	/** The settings file that was read, if any. */
	settingsFilePath: string | undefined
	/** Non-fatal problems worth showing the user. */
	warnings: string[]
}

async function fileExists(filePath: string): Promise<boolean> {
	try {
		await fs.access(filePath)
		return true
	} catch {
		return false
	}
}

/**
 * Reads a settings file. Returns `undefined` when the file does not exist.
 */
async function loadSettingsFile(filePath: string): Promise<PartialYankiPluginSettings | undefined> {
	let contents: string
	try {
		contents = await fs.readFile(filePath, 'utf8')
	} catch (error) {
		if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
			return undefined
		}

		throw error
	}

	try {
		return JSON.parse(contents) as PartialYankiPluginSettings
	} catch (error) {
		throw new Error(`Could not parse settings file ${filePath}`, { cause: error })
	}
}

/**
 * Merges partial settings over the defaults, one level deep inside each group
 * of settings, so a file only needs to mention the fields it changes.
 */
function mergeSettings(
	base: YankiPluginSettings,
	override: PartialYankiPluginSettings | undefined,
): YankiPluginSettings {
	if (override === undefined) {
		return base
	}

	return {
		ankiConnect: { ...base.ankiConnect, ...override.ankiConnect },
		folders: override.folders ?? base.folders,
		ignoreFolderNotes: override.ignoreFolderNotes ?? base.ignoreFolderNotes,
		manageFilenames: { ...base.manageFilenames, ...override.manageFilenames },
		namespace: override.namespace ?? base.namespace,
		showAdvancedSettings: override.showAdvancedSettings ?? base.showAdvancedSettings,
		stats: base.stats,
		sync: { ...base.sync, ...override.sync },
		verboseNotices: override.verboseNotices ?? base.verboseNotices,
	}
}

/**
 * Applies command line flags on top of the settings.
 */
function applyArguments(
	settings: YankiPluginSettings,
	argv: ArgumentsCamelCase<CliArguments>,
): YankiPluginSettings {
	const merged = structuredClone(settings)

	if (argv.folder !== undefined && argv.folder.length > 0) {
		merged.folders = argv.folder
	}

	if (argv.ignoreFolderNotes !== undefined) {
		merged.ignoreFolderNotes = argv.ignoreFolderNotes
	}

	if (argv.ankiConnect !== undefined) {
		const hostAndPort = urlToHostAndPort(argv.ankiConnect)
		if (hostAndPort === undefined) {
			throw new Error(`Invalid AnkiConnect URL: "${argv.ankiConnect}"`)
		}

		merged.ankiConnect.host = hostAndPort.host
		merged.ankiConnect.port = hostAndPort.port
	}

	if (argv.ankiKey !== undefined) {
		merged.ankiConnect.key = argv.ankiKey
	}

	if (argv.ankiWeb !== undefined) {
		merged.sync.pushToAnkiWeb = argv.ankiWeb
	}

	if (argv.syncMedia !== undefined) {
		merged.sync.mediaMode = argv.syncMedia
	}

	if (argv.manageFilenames !== undefined) {
		merged.manageFilenames.autoRenameTrigger =
			argv.manageFilenames === 'off' ? 'off' : 'before-sync'
		if (argv.manageFilenames !== 'off') {
			merged.manageFilenames.mode = argv.manageFilenames
		}
	}

	if (argv.maxFilenameLength !== undefined) {
		merged.manageFilenames.maxLength = argv.maxFilenameLength
	}

	return merged
}

/**
 * Picks the namespace, preferring explicit choices over derived ones.
 *
 * A vault's namespace has to match the plugin's namespace, otherwise the plugin
 * and the CLI would each create their own copies of the same notes in Anki.
 */
function resolveNamespace(
	argv: ArgumentsCamelCase<CliArguments>,
	directory: SyncDirectory,
	settings: YankiPluginSettings,
	settingsFilePath: string | undefined,
): { namespace: string; source: NamespaceSource; warnings: string[] } {
	const warnings: string[] = []

	if (argv.namespace !== undefined) {
		return { namespace: argv.namespace, source: 'the --namespace option', warnings }
	}

	if (settingsFilePath !== undefined) {
		return { namespace: settings.namespace, source: 'settings file', warnings }
	}

	if (directory.obsidianId !== undefined) {
		return { namespace: settings.namespace, source: 'the Obsidian vault ID', warnings }
	}

	// Nothing identifies the notes in Anki beyond the directory itself.
	const fallback = `Yanki - ${directory.name}`
	if (directory.isObsidianVault) {
		warnings.push(
			`Could not determine the Obsidian ID of this vault, so its Anki notes will use the namespace "${fallback}". If this vault is also synced with the Yanki Obsidian plugin, pass --namespace with the namespace from that vault's plugin settings, otherwise the same notes will appear twice in Anki.`,
		)
	}

	return { namespace: fallback, source: 'directory name', warnings }
}

/**
 * Combines the built-in defaults, an optional settings file, and command line
 * flags into the settings the commands act on.
 */
export async function resolveSettings(
	argv: ArgumentsCamelCase<CliArguments>,
	directory: SyncDirectory,
): Promise<ResolvedSettings> {
	const settingsFilePath =
		argv.config ??
		(directory.pluginSettingsPath !== undefined && (await fileExists(directory.pluginSettingsPath))
			? directory.pluginSettingsPath
			: undefined)

	const fileSettings =
		settingsFilePath === undefined ? undefined : await loadSettingsFile(settingsFilePath)

	if (settingsFilePath !== undefined && fileSettings === undefined) {
		throw new Error(`Settings file not found: ${settingsFilePath}`)
	}

	const settings = applyArguments(
		mergeSettings(getYankiPluginDefaultSettings(directory.obsidianId), fileSettings),
		argv,
	)

	const namespace = resolveNamespace(argv, directory, settings, settingsFilePath)
	const warnings = [...namespace.warnings]

	if (!validateNamespace(namespace.namespace)) {
		warnings.push(
			`The namespace "${namespace.namespace}" contains characters that Anki does not allow, and will be used as "${sanitizeNamespace(namespace.namespace)}".`,
		)
	}

	if (settings.folders.length === 0) {
		if (directory.isObsidianVault) {
			throw new Error(
				'No flashcard folders are configured for this vault. Pass --folder with a path relative to the vault root (use / for the whole vault), or configure folders in the Yanki plugin settings tab.',
			)
		}

		// A plain directory of notes is synced as a whole.
		settings.folders = ['.']
	}

	return {
		folders: settings.folders,
		namespace: namespace.namespace,
		namespaceSource: namespace.source,
		settings: { ...settings, namespace: namespace.namespace },
		settingsFilePath,
		warnings,
	}
}
