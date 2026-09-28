import type { InferredOptionTypes, Options } from 'yargs'

/**
 * Flags shared by every command. Nothing here declares a default: an absent
 * flag must stay `undefined` so the value from the settings file can win.
 */
export const sharedOptions = {
	'anki-auto-launch': {
		describe: 'Open the Anki desktop application if it is not already running. (macOS only.)',
		type: 'boolean',
	},
	'anki-connect': {
		describe:
			'Host and port of the AnkiConnect server, e.g. http://127.0.0.1:8765. Defaults to the value in the settings file.',
		type: 'string',
	},
	'anki-key': {
		describe: 'AnkiConnect API key, if your AnkiConnect configuration requires one.',
		type: 'string',
	},
	'anki-web': {
		describe:
			'Sync to AnkiWeb after syncing locally. This is the equivalent of pressing the "Sync" button in the Anki app.',
		type: 'boolean',
	},
	config: {
		describe:
			'Path to a settings file. Defaults to the Yanki plugin settings inside an Obsidian vault, when present.',
		type: 'string',
	},
	folder: {
		array: true,
		describe:
			'Flashcard folder to sync, relative to the synced directory. Repeatable. Overrides the folders in the settings file. Use / for the entire directory.',
		type: 'string',
	},
	'ignore-folder-notes': {
		describe: 'Skip notes that share the name of their parent folder.',
		type: 'boolean',
	},
	json: {
		describe: 'Print the result as JSON.',
		type: 'boolean',
	},
	'max-filename-length': {
		describe: 'Maximum length of an automatically managed note file name, in characters.',
		type: 'number',
	},
	namespace: {
		describe:
			'Advanced option for managing multiple synchronization groups. Case insensitive. See the readme for more information.',
		type: 'string',
	},
	'strict-line-breaks': {
		describe:
			'Treat single newlines in Markdown as line breaks. Defaults to the Obsidian vault setting.',
		type: 'boolean',
	},
	'sync-media': {
		choices: ['off', 'local', 'remote', 'all'] as const,
		describe:
			'Sync image, video, and audio assets to Anki. "local" syncs vault assets, "remote" syncs hot-linked URLs, "all" syncs both, and "off" skips asset syncing entirely.',
		type: 'string' as const,
	},
	verbose: {
		describe: 'Print additional details about the sync.',
		type: 'boolean',
	},
} satisfies Record<string, Options>

export const syncOptions = {
	...sharedOptions,
	'dry-run': {
		alias: 'd',
		describe:
			'Run without making any changes to the Anki database. See a report of what would have been done.',
		type: 'boolean',
	},
	'manage-filenames': {
		choices: ['off', 'prompt', 'response'] as const,
		describe:
			'Rename note files to match their content before syncing. "prompt" names notes after the front of the card, "response" after the back, and "off" disables renaming.',
		type: 'string' as const,
	},
} satisfies Record<string, Options>

export const renameOptions = {
	...sharedOptions,
	'dry-run': {
		alias: 'd',
		describe: 'Show what would be renamed without touching any files.',
		type: 'boolean',
	},
	'manage-filenames': {
		choices: ['off', 'prompt', 'response'] as const,
		describe:
			'Portion of the note to derive the file name from. Defaults to the name mode in the settings file.',
		type: 'string' as const,
	},
} satisfies Record<string, Options>

export const directoryOption = {
	describe:
		'Path to the directory or Obsidian vault to sync. Defaults to the current working directory.',
	type: 'string',
} satisfies Options

/** Every flag the CLI understands, with the value types yargs derives from them. */
export type CliArguments = InferredOptionTypes<typeof sharedOptions> & {
	directory?: string
	dryRun?: boolean
	manageFilenames?: 'off' | 'prompt' | 'response'
}
