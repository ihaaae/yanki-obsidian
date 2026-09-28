import type { SyncFilesOptions } from 'yanki'
import type { ArgumentsCamelCase } from 'yargs'
import fs from 'node:fs'
import process from 'node:process'
import plur from 'plur'
import { formatSyncFilesResult, hostAndPortToUrl, renameFiles, syncFiles } from 'yanki'
import yargs from 'yargs'
import { hideBin } from 'yargs/helpers'
import type { CliArguments } from './options'
import type { ResolvedSettings } from './settings'
import type { SyncDirectory } from './vault'
import { directoryOption, renameOptions, sharedOptions, syncOptions } from './options'
import { resolveSettings } from './settings'
import { findNotePaths, resolveSyncDirectory } from './vault'

const ANKI_CONNECT_VERSION = 6

function getVersion(): string {
	try {
		const { version } = JSON.parse(
			fs.readFileSync(new URL('../package.json', import.meta.url), 'utf8'),
		) as { version: string }
		return version
	} catch {
		return 'unknown'
	}
}

function writeStdout(message: string): void {
	process.stdout.write(message.endsWith('\n') ? message : `${message}\n`)
}

function writeStderr(message: string): void {
	process.stderr.write(message.endsWith('\n') ? message : `${message}\n`)
}

/**
 * Describes where the notes came from and how they will be synced.
 */
function formatContext(
	directory: SyncDirectory,
	resolved: ResolvedSettings,
	noteCount: number,
): string {
	const { settings } = resolved

	return [
		`Directory: ${directory.rootPath}${directory.isObsidianVault ? ' (Obsidian vault)' : ''}`,
		...[
			resolved.settingsFilePath === undefined
				? undefined
				: `Settings: ${resolved.settingsFilePath}`,
			`Namespace: ${resolved.namespace} (from ${resolved.namespaceSource})`,
			`Flashcard folders: ${resolved.folders.join(', ')}`,
			`Notes to sync: ${String(noteCount)}`,
			`AnkiConnect: ${hostAndPortToUrl(settings.ankiConnect.host, settings.ankiConnect.port)}`,
			`Sync media assets: ${settings.sync.mediaMode}`,
			`Push to AnkiWeb: ${settings.sync.pushToAnkiWeb ? 'yes' : 'no'}`,
			`Automatic note names: ${settings.manageFilenames.autoRenameTrigger === 'off' ? 'off' : settings.manageFilenames.mode}`,
			`Ignore folder notes: ${settings.ignoreFolderNotes ? 'yes' : 'no'}`,
		].filter((line) => line !== undefined),
	].join('\n')
}

/**
 * Resolves the directory, settings, and note selection shared by every command,
 * reporting any warnings along the way.
 */
async function prepare(argv: ArgumentsCamelCase<CliArguments>): Promise<{
	directory: SyncDirectory
	notePaths: string[]
	resolved: ResolvedSettings
}> {
	const directory = await resolveSyncDirectory(argv.directory ?? process.cwd())
	const resolved = await resolveSettings(argv, directory)

	for (const warning of resolved.warnings) {
		writeStderr(`Warning: ${warning}`)
	}

	return {
		directory,
		notePaths: findNotePaths(
			directory.allFilePaths,
			directory.rootPath,
			resolved.folders,
			resolved.settings.ignoreFolderNotes,
		),
		resolved,
	}
}

/**
 * Options common to the sync and rename actions. The Yanki library provides its
 * own Node file system and fetch implementations, so none are passed here.
 */
function getSharedFileOptions(
	argv: ArgumentsCamelCase<CliArguments>,
	directory: SyncDirectory,
	resolved: ResolvedSettings,
	manageFilenames: 'off' | 'prompt' | 'response',
) {
	return {
		allFilePaths: directory.allFilePaths,
		basePath: directory.rootPath,
		dryRun: argv.dryRun === true,
		// The library falls back to Node's file system and global fetch.
		fetchAdapter: undefined,
		fileAdapter: undefined,
		manageFilenames,
		maxFilenameLength: resolved.settings.manageFilenames.maxLength,
		namespace: resolved.namespace,
		obsidianVault: directory.isObsidianVault ? directory.name : undefined,
		strictLineBreaks: argv.strictLineBreaks ?? directory.strictLineBreaks ?? true,
		syncMediaAssets: resolved.settings.sync.mediaMode,
	}
}

function getSyncFilesOptions(
	argv: ArgumentsCamelCase<CliArguments>,
	directory: SyncDirectory,
	resolved: ResolvedSettings,
): SyncFilesOptions {
	const { ankiConnect } = resolved.settings
	const { autoRenameTrigger, mode } = resolved.settings.manageFilenames

	return {
		...getSharedFileOptions(argv, directory, resolved, autoRenameTrigger === 'off' ? 'off' : mode),
		ankiConnectOptions: {
			autoLaunch: argv.ankiAutoLaunch === true,
			// The library falls back to Node's global fetch.
			fetchAdapter: undefined,
			host: ankiConnect.host,
			key: ankiConnect.key,
			port: ankiConnect.port,
			version: ANKI_CONNECT_VERSION,
		},
		ankiWeb: resolved.settings.sync.pushToAnkiWeb,
		checkDatabase: true,
		strictMatching: false,
	}
}

async function syncCommand(argv: ArgumentsCamelCase<CliArguments>): Promise<number> {
	const { directory, notePaths, resolved } = await prepare(argv)

	if (notePaths.length === 0) {
		writeStderr(`No Markdown notes found in ${resolved.folders.join(', ')}.`)
		return 1
	}

	if (argv.verbose === true && argv.json !== true) {
		writeStdout(formatContext(directory, resolved, notePaths.length))
	}

	const result = await syncFiles(notePaths, getSyncFilesOptions(argv, directory, resolved))
	const ankiUnreachable = result.synced.some(({ action }) => action === 'ankiUnreachable')

	if (argv.json === true) {
		writeStdout(JSON.stringify(result, undefined, 2))
	} else if (ankiUnreachable) {
		writeStderr(
			'Could not connect to Anki. Make sure the Anki desktop application is running, and that the AnkiConnect add-on is installed and configured.',
		)
	} else {
		writeStdout(formatSyncFilesResult(result, argv.verbose === true))
	}

	return ankiUnreachable ? 1 : 0
}

async function renameCommand(argv: ArgumentsCamelCase<CliArguments>): Promise<number> {
	const { directory, notePaths, resolved } = await prepare(argv)

	if (notePaths.length === 0) {
		writeStderr(`No Markdown notes found in ${resolved.folders.join(', ')}.`)
		return 1
	}

	// An explicit rename request acts on the configured name mode, even when
	// automatic renaming is switched off.
	const manageFilenames = argv.manageFilenames ?? resolved.settings.manageFilenames.mode

	if (manageFilenames === 'off') {
		writeStderr(
			'Note file renaming is disabled. Pass --manage-filenames with "prompt" or "response" to rename files.',
		)
		return 1
	}

	const result = await renameFiles(notePaths, {
		...getSharedFileOptions(argv, directory, resolved, manageFilenames),
		allFilePaths: [],
		syncMediaAssets: 'off',
	})

	const renamed = result.notes.filter(
		({ filePath, filePathOriginal }) => filePath !== filePathOriginal,
	)

	if (argv.json === true) {
		writeStdout(
			JSON.stringify(
				{
					dryRun: result.dryRun,
					renamed: renamed.map(({ filePath, filePathOriginal }) => ({
						filePath,
						filePathOriginal,
					})),
				},
				undefined,
				2,
			),
		)
	} else if (renamed.length === 0) {
		writeStdout('All note file names are up to date.')
	} else {
		writeStdout(
			[
				`${result.dryRun ? 'Would rename' : 'Renamed'} ${String(renamed.length)} ${plur('note', renamed.length)}:`,
				...renamed.map(({ filePath, filePathOriginal }) => `  ${filePathOriginal} → ${filePath}`),
			].join('\n'),
		)
	}

	return 0
}

async function configCommand(argv: ArgumentsCamelCase<CliArguments>): Promise<number> {
	const { directory, notePaths, resolved } = await prepare(argv)

	if (argv.json === true) {
		writeStdout(
			JSON.stringify(
				{
					ankiConnect: hostAndPortToUrl(
						resolved.settings.ankiConnect.host,
						resolved.settings.ankiConnect.port,
					),
					// The AnkiConnect key is deliberately omitted, it's a secret.
					directory: directory.rootPath,
					folders: resolved.folders,
					ignoreFolderNotes: resolved.settings.ignoreFolderNotes,
					isObsidianVault: directory.isObsidianVault,
					manageFilenames:
						resolved.settings.manageFilenames.autoRenameTrigger === 'off'
							? 'off'
							: resolved.settings.manageFilenames.mode,
					namespace: resolved.namespace,
					namespaceSource: resolved.namespaceSource,
					notes: notePaths.length,
					pushToAnkiWeb: resolved.settings.sync.pushToAnkiWeb,
					settingsFile: resolved.settingsFilePath,
					strictLineBreaks: argv.strictLineBreaks ?? directory.strictLineBreaks ?? true,
					syncMedia: resolved.settings.sync.mediaMode,
				},
				undefined,
				2,
			),
		)
	} else {
		writeStdout(formatContext(directory, resolved, notePaths.length))
	}

	return 0
}

// Yargs does not await asynchronous command handlers, so the running command is
// tracked here and awaited after parsing.
let running: Promise<void> | undefined

/**
 * Runs a command and records its exit code, turning any thrown error into a
 * message and a failure code.
 */
async function run(command: () => Promise<number>): Promise<void> {
	try {
		process.exitCode = await command()
	} catch (error) {
		writeStderr(`Error: ${error instanceof Error ? error.message : String(error)}`)
		process.exitCode = 1
	}
}

await yargs(hideBin(process.argv))
	.scriptName('yanki-obsidian')
	.usage('$0 [command] [directory] [options]')
	.example([
		['$0 sync ~/Notes/Vault', 'Sync the flashcard folders configured in an Obsidian vault'],
		['$0 sync ~/Notes/Vault --folder Flashcards --dry-run', 'Preview a sync of a single folder'],
		[
			'$0 sync ~/markdown --namespace "Yanki - markdown"',
			'Sync a plain directory of Markdown notes',
		],
		['$0 rename ~/Notes/Vault', 'Rename note files to match their content'],
	])
	.command(
		['$0 [directory]', 'sync [directory]'],
		'Sync Markdown notes to Anki. This is the default command.',
		(builder) => builder.positional('directory', directoryOption).options(syncOptions),
		(argv) => {
			running = run(async () => syncCommand(argv))
		},
	)
	.command(
		'rename [directory]',
		'Rename note files to match their content, without syncing to Anki.',
		(builder) => builder.positional('directory', directoryOption).options(renameOptions),
		(argv) => {
			running = run(async () => renameCommand(argv))
		},
	)
	.command(
		'config [directory]',
		'Print the settings and notes that a sync would use.',
		(builder) => builder.positional('directory', directoryOption).options(sharedOptions),
		(argv) => {
			running = run(async () => configCommand(argv))
		},
	)
	.strict()
	.help()
	.version(getVersion())
	.parseAsync()

await running
