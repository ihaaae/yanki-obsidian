import fs from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'

/**
 * Directory names that never contain notes and are never link targets, even
 * when they aren't hidden. Hidden entries are skipped separately, mirroring
 * Obsidian's own vault index.
 */
const IGNORED_DIRECTORY_NAMES = new Set(['node_modules'])

/**
 * What the CLI knows about the directory it's syncing, gathered from the file
 * system rather than from a running Obsidian application.
 */
export type SyncDirectory = {
	/** Every non-hidden file in the directory, used to resolve wiki-style links. */
	allFilePaths: string[]
	/**
	 * Whether the directory is an Obsidian vault, i.e. it contains a `.obsidian`
	 * folder.
	 */
	isObsidianVault: boolean
	/** Directory name, used for `obsidian://` links. */
	name: string
	/** Obsidian's internal ID for the vault, when Obsidian has opened it before. */
	obsidianId: string | undefined
	/** The Yanki plugin's settings file, when the directory is an Obsidian vault. */
	pluginSettingsPath: string | undefined
	/** Absolute path to the synced directory. */
	rootPath: string
	/** Obsidian's "Strict line breaks" setting, when the directory is a vault. */
	strictLineBreaks: boolean | undefined
}

type ObsidianConfig = {
	vaults?: Record<string, { path?: string }>
}

async function isDirectory(directoryPath: string): Promise<boolean> {
	try {
		const stats = await fs.stat(directoryPath)
		return stats.isDirectory()
	} catch {
		return false
	}
}

/**
 * Walks a directory tree, skipping hidden entries and ignored directories.
 */
async function listFiles(directoryPath: string): Promise<string[]> {
	const entries = await fs.readdir(directoryPath, { withFileTypes: true })
	const filePaths: string[] = []

	for (const entry of entries) {
		if (entry.name.startsWith('.') || IGNORED_DIRECTORY_NAMES.has(entry.name)) {
			continue
		}

		const entryPath = path.join(directoryPath, entry.name)

		if (entry.isDirectory()) {
			filePaths.push(...(await listFiles(entryPath)))
		} else if (entry.isFile()) {
			filePaths.push(entryPath)
		}
	}

	return filePaths
}

/**
 * Obsidian's "Strict line breaks" setting. Obsidian treats single newlines as
 * line breaks unless the setting is enabled, so an absent value means `false`.
 */
async function readStrictLineBreaks(vaultPath: string): Promise<boolean> {
	try {
		const appConfig = JSON.parse(
			await fs.readFile(path.join(vaultPath, '.obsidian', 'app.json'), 'utf8'),
		) as { strictLineBreaks?: boolean }
		return appConfig.strictLineBreaks ?? false
	} catch {
		return false
	}
}

/**
 * Location of Obsidian's global configuration, which maps vault IDs to vault
 * paths. Mirrors Obsidian's own platform conventions.
 */
function getObsidianConfigDirectory(): string {
	const platformDirectories: Partial<Record<NodeJS.Platform, string>> = {
		darwin: path.join(os.homedir(), 'Library', 'Application Support', 'obsidian'),
		win32: path.join(
			process.env.APPDATA ?? path.join(os.homedir(), 'AppData', 'Roaming'),
			'obsidian',
		),
	}

	return (
		platformDirectories[process.platform] ??
		path.join(process.env.XDG_CONFIG_HOME ?? path.join(os.homedir(), '.config'), 'obsidian')
	)
}

/**
 * Finds the Obsidian vault ID that matches a vault path.
 *
 * The plugin derives its Anki namespace from this ID, so resolving it lets the
 * CLI produce the same namespace as the plugin even when the plugin's settings
 * file isn't available.
 */
async function findObsidianVaultId(vaultPath: string): Promise<string | undefined> {
	try {
		const config = JSON.parse(
			await fs.readFile(path.join(getObsidianConfigDirectory(), 'obsidian.json'), 'utf8'),
		) as ObsidianConfig
		const expectedPath = path.resolve(vaultPath)
		const caseInsensitive = process.platform !== 'linux'
		const vaults = Object.entries(config.vaults ?? {})

		for (const [id, vault] of vaults) {
			if (vault.path === undefined) {
				continue
			}

			const candidatePath = path.resolve(vault.path)
			const matches = caseInsensitive
				? candidatePath.toLowerCase() === expectedPath.toLowerCase()
				: candidatePath === expectedPath

			if (matches) {
				return id
			}
		}
	} catch {
		// Obsidian may not be installed, or may never have opened this vault.
	}

	return undefined
}

/**
 * Inspects a directory of Markdown notes, detecting Obsidian vault metadata
 * when it's present.
 */
export async function resolveSyncDirectory(directoryPath: string): Promise<SyncDirectory> {
	const rootPath = path.resolve(directoryPath)

	if (!(await isDirectory(rootPath))) {
		throw new Error(`Not a directory: ${rootPath}`)
	}

	const isObsidianVault = await isDirectory(path.join(rootPath, '.obsidian'))

	return {
		allFilePaths: await listFiles(rootPath),
		isObsidianVault,
		name: path.basename(rootPath),
		obsidianId: isObsidianVault ? await findObsidianVaultId(rootPath) : undefined,
		pluginSettingsPath: isObsidianVault
			? path.join(rootPath, '.obsidian', 'plugins', 'yanki', 'data.json')
			: undefined,
		rootPath,
		strictLineBreaks: isObsidianVault ? await readStrictLineBreaks(rootPath) : undefined,
	}
}

/**
 * Resolves a vault-relative folder path, as stored in the plugin's settings, to
 * an absolute path. `/`, `.`, and an empty string all refer to the root.
 */
function resolveFolderPath(rootPath: string, folder: string): string {
	const segments = folder
		.replaceAll('\\', '/')
		.split('/')
		.filter((segment) => segment.length > 0 && segment !== '.')

	return segments.length === 0 ? rootPath : path.join(rootPath, ...segments)
}

/**
 * Selects the notes to sync, mirroring the plugin's watched folder behavior:
 * recursive, Markdown only, and optionally skipping folder notes.
 */
export function findNotePaths(
	allFilePaths: string[],
	rootPath: string,
	folders: string[],
	ignoreFolderNotes: boolean,
): string[] {
	const folderPaths = folders.map((folder) => resolveFolderPath(rootPath, folder))
	const notePaths = new Set<string>()

	for (const filePath of allFilePaths) {
		if (path.extname(filePath).toLowerCase() !== '.md') {
			continue
		}

		if (folderPaths.every((folderPath) => !filePath.startsWith(folderPath + path.sep))) {
			continue
		}

		if (
			ignoreFolderNotes &&
			path.basename(path.dirname(filePath)) === path.basename(filePath, path.extname(filePath))
		) {
			continue
		}

		notePaths.add(filePath)
	}

	return [...notePaths].toSorted((a, b) => a.localeCompare(b))
}
