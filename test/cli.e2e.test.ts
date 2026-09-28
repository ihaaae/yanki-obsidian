import { execFile } from 'node:child_process'
import { randomUUID } from 'node:crypto'
import fs from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { promisify } from 'node:util'
import { expect, inject, test } from 'vitest'
import type { AnkiConnection } from './support/anki'
import { ankiRequest } from './support/anki'

// These tests run the built command line interface as a subprocess, against the
// disposable Anki collection and a throwaway copy of the desktop test vault.
// They cover the CLI boundary: settings resolution, note discovery, the file
// system, and a real sync with no Obsidian involved.

const execFileAsync = promisify(execFile) // eslint-disable-line ts/strict-void-return -- execFile supports promisify and also returns its child process.

const cliPath = path.resolve('dist/cli.js')
const fixtureVaultPath = path.resolve('test/vault')

type CliResult = {
	code: number
	stderr: string
	stdout: string
}

type SyncReport = {
	synced: Array<{ action: string; filePath: string | undefined; note: { noteId: number } }>
}

/**
 * Runs the CLI, returning its output instead of throwing on a failure exit
 * code.
 */
async function runCli(args: string[]): Promise<CliResult> {
	try {
		const { stderr, stdout } = await execFileAsync(process.execPath, [cliPath, ...args])
		return { code: 0, stderr, stdout }
	} catch (error) {
		const { code, stderr, stdout } = error as {
			code?: number | string
			stderr?: string
			stdout?: string
		}
		return {
			code: typeof code === 'number' ? code : 1,
			stderr: stderr ?? '',
			stdout: stdout ?? '',
		}
	}
}

/**
 * Copies the desktop test vault to a temporary directory and writes the
 * settings file the Yanki plugin would have written for the test Anki
 * instance.
 */
async function createVaultFixture(
	connection: AnkiConnection,
	namespace: string,
	settings: Record<string, unknown> = {},
): Promise<string> {
	const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'yanki-cli-'))
	const vaultPath = path.join(directory, 'Vault')
	await fs.cp(fixtureVaultPath, vaultPath, { recursive: true })
	const pluginPath = path.join(vaultPath, '.obsidian', 'plugins', 'yanki')
	await fs.mkdir(pluginPath, { recursive: true })
	await fs.writeFile(
		path.join(pluginPath, 'data.json'),
		JSON.stringify(
			{
				ankiConnect: {
					host: 'http://127.0.0.1',
					key: connection.key,
					port: connection.port,
				},
				folders: ['Anki'],
				namespace,
				sync: { pushToAnkiWeb: false },
				...settings,
			},
			undefined,
			2,
		),
	)
	return vaultPath
}

async function findNoteIds(connection: AnkiConnection, namespace: string): Promise<number[]> {
	return ankiRequest<number[]>(connection, 'findNotes', {
		query: `"YankiNamespace:${namespace}"`,
	})
}

async function readNoteId(filePath: string): Promise<number> {
	const markdown = await fs.readFile(filePath, 'utf8')
	return Number(/^noteId: (\d+)$/mu.exec(markdown)?.[1])
}

async function readDecks(connection: AnkiConnection, namespace: string): Promise<string[]> {
	const cards = await ankiRequest<number[]>(connection, 'findCards', {
		query: `"YankiNamespace:${namespace}"`,
	})
	const decks = await ankiRequest<Record<string, number[]>>(connection, 'getDecks', { cards })
	return Object.keys(decks).toSorted()
}

test('syncs an Obsidian vault using the plugin settings file', async () => {
	const connection = inject('anki')
	const namespace = `Yanki CLI ${randomUUID()}`
	const vaultPath = await createVaultFixture(connection, namespace)

	try {
		const first = await runCli(['sync', vaultPath, '--json'])
		expect(first.stderr).toBe('')
		expect(first.code).toBe(0)

		const report = JSON.parse(first.stdout) as SyncReport
		expect(report.synced).toHaveLength(4)
		expect(report.synced.every(({ action }) => action === 'created')).toBe(true)

		const noteIds = await findNoteIds(connection, namespace)
		expect(noteIds).toHaveLength(4)
		expect(await readDecks(connection, namespace)).toEqual([
			'Animals',
			'Animals::Biped',
			'Animals::Quadruped',
			'Non-living things',
		])

		// The frontmatter of every synced note records its Anki note ID.
		for (const { filePath, note } of report.synced) {
			expect(filePath).toBeDefined()
			expect(await readNoteId(filePath!)).toBe(note.noteId)
		}

		// A second sync must be a no-op, preserving existing note IDs.
		const second = await runCli(['sync', vaultPath, '--json'])
		expect(second.code).toBe(0)
		const secondReport = JSON.parse(second.stdout) as SyncReport
		expect(secondReport.synced.every(({ action }) => action === 'unchanged')).toBe(true)
		expect(await findNoteIds(connection, namespace)).toEqual(noteIds)

		// Notes deleted locally are deleted from Anki on the next sync.
		const removed = report.synced.find(({ filePath }) => filePath?.endsWith('Rock.md'))
		await fs.rm(removed!.filePath!)
		const third = await runCli(['sync', vaultPath, '--json'])
		expect(third.code).toBe(0)
		const thirdReport = JSON.parse(third.stdout) as SyncReport
		expect(thirdReport.synced.filter(({ action }) => action === 'deleted')).toHaveLength(1)
		expect(thirdReport.synced.filter(({ action }) => action === 'unchanged')).toHaveLength(3)
		expect(await findNoteIds(connection, namespace)).toHaveLength(3)
	} finally {
		await fs.rm(path.dirname(vaultPath), { force: true, recursive: true })
	}
})

test('syncs a directory of notes with no Obsidian vault or settings file', async () => {
	const connection = inject('anki')
	const namespace = `Yanki CLI ${randomUUID()}`
	const vaultPath = await createVaultFixture(connection, namespace)
	const notesPath = path.join(vaultPath, 'Anki')

	try {
		const result = await runCli([
			'sync',
			notesPath,
			'--namespace',
			namespace,
			'--anki-connect',
			`http://127.0.0.1:${String(connection.port)}`,
			'--anki-key',
			connection.key,
			'--no-anki-web',
			'--json',
		])
		expect(result.stderr).toBe('')
		expect(result.code).toBe(0)
		expect((JSON.parse(result.stdout) as SyncReport).synced).toHaveLength(4)
		expect(await findNoteIds(connection, namespace)).toHaveLength(4)
	} finally {
		await fs.rm(path.dirname(vaultPath), { force: true, recursive: true })
	}
})

test('reports the settings and note selection without syncing', async () => {
	const connection = inject('anki')
	const namespace = `Yanki CLI ${randomUUID()}`
	const vaultPath = await createVaultFixture(connection, namespace)

	try {
		const result = await runCli(['config', vaultPath, '--json'])
		expect(result.stderr).toBe('')
		expect(result.code).toBe(0)
		expect(JSON.parse(result.stdout)).toMatchObject({
			folders: ['Anki'],
			isObsidianVault: true,
			namespace,
			namespaceSource: 'settings file',
			notes: 4,
		})

		// Command line folders override the ones in the settings file.
		const overridden = await runCli(['config', vaultPath, '--folder', '/', '--json'])
		expect(JSON.parse(overridden.stdout)).toMatchObject({ folders: ['/'], notes: 5 })
	} finally {
		await fs.rm(path.dirname(vaultPath), { force: true, recursive: true })
	}
})

test('previews a sync without changing Anki', async () => {
	const connection = inject('anki')
	const namespace = `Yanki CLI ${randomUUID()}`
	const vaultPath = await createVaultFixture(connection, namespace)

	try {
		const result = await runCli(['sync', vaultPath, '--dry-run', '--json'])
		expect(result.code).toBe(0)
		const report = JSON.parse(result.stdout) as SyncReport & { dryRun: boolean }
		expect(report.dryRun).toBe(true)
		expect(report.synced).toHaveLength(4)
		expect(await findNoteIds(connection, namespace)).toHaveLength(0)

		// The dry run must not have written note IDs into the local notes.
		expect(
			await fs.readFile(path.join(vaultPath, 'Anki/Non-living things/Rock.md'), 'utf8'),
		).not.toContain('noteId')
	} finally {
		await fs.rm(path.dirname(vaultPath), { force: true, recursive: true })
	}
})

test('syncs only the folders given on the command line', async () => {
	const connection = inject('anki')
	const namespace = `Yanki CLI ${randomUUID()}`
	const vaultPath = await createVaultFixture(connection, namespace)

	try {
		const result = await runCli(['sync', vaultPath, '--folder', 'Anki/Animals', '--json'])
		expect(result.stderr).toBe('')
		expect(result.code).toBe(0)
		expect((JSON.parse(result.stdout) as SyncReport).synced).toHaveLength(3)
		expect(await readDecks(connection, namespace)).toEqual([
			'Animals',
			'Animals::Biped',
			'Animals::Quadruped',
		])
	} finally {
		await fs.rm(path.dirname(vaultPath), { force: true, recursive: true })
	}
})

test('renames note files to match their content', async () => {
	const connection = inject('anki')
	const namespace = `Yanki CLI ${randomUUID()}`
	const vaultPath = await createVaultFixture(connection, namespace, {
		manageFilenames: { mode: 'prompt' },
	})
	const notePath = path.join(vaultPath, 'Anki/Animals/Biped/Untitled.md')
	const renamedPath = path.join(vaultPath, 'Anki/Animals/Biped/A chicken.md')
	await fs.writeFile(notePath, 'A chicken\n\n---\n\nA biped.\n')

	try {
		const disabled = await runCli(['rename', vaultPath, '--manage-filenames', 'off'])
		expect(disabled.code).toBe(1)
		expect(disabled.stderr).toContain('Note file renaming is disabled')

		const dryRun = await runCli(['rename', vaultPath, '--dry-run', '--json'])
		expect(dryRun.code).toBe(0)
		const dryRunReport = JSON.parse(dryRun.stdout) as {
			dryRun: boolean
			renamed: Array<{ filePath: string; filePathOriginal: string }>
		}
		expect(dryRunReport.dryRun).toBe(true)
		expect(dryRunReport.renamed).toContainEqual({
			filePath: renamedPath,
			filePathOriginal: notePath,
		})
		expect(await fs.readFile(notePath, 'utf8')).toContain('A chicken')

		const result = await runCli(['rename', vaultPath])
		expect(result.stderr).toBe('')
		expect(result.code).toBe(0)
		expect(result.stdout).toContain('Renamed 2 notes')
		expect(result.stdout).toContain(`${notePath} → ${renamedPath}`)
		expect(await fs.readFile(renamedPath, 'utf8')).toContain('A chicken')
		await expect(fs.access(notePath)).rejects.toThrow()
	} finally {
		await fs.rm(path.dirname(vaultPath), { force: true, recursive: true })
	}
})

test('reads settings from an explicit config file', async () => {
	const connection = inject('anki')
	const vaultPath = await createVaultFixture(connection, `Yanki CLI ${randomUUID()}`)
	const configPath = path.join(path.dirname(vaultPath), 'cli-settings.json')
	const namespace = `Yanki CLI config ${randomUUID()}`
	await fs.writeFile(
		configPath,
		JSON.stringify({ folders: ['Anki/Animals'], namespace }, undefined, 2),
	)

	try {
		const result = await runCli(['config', vaultPath, '--config', configPath, '--json'])
		expect(result.stderr).toBe('')
		expect(result.code).toBe(0)
		expect(JSON.parse(result.stdout)).toMatchObject({
			folders: ['Anki/Animals'],
			namespace,
			namespaceSource: 'settings file',
			notes: 3,
			settingsFile: configPath,
		})
	} finally {
		await fs.rm(path.dirname(vaultPath), { force: true, recursive: true })
	}
})

test('warns when a vault has neither a settings file nor a known vault ID', async () => {
	const connection = inject('anki')
	const vaultPath = await createVaultFixture(connection, `Yanki CLI ${randomUUID()}`)
	await fs.rm(path.join(vaultPath, '.obsidian', 'plugins'), { force: true, recursive: true })

	try {
		const result = await runCli(['config', vaultPath, '--folder', 'Anki', '--json'])
		expect(result.code).toBe(0)
		expect(result.stderr).toContain('Could not determine the Obsidian ID of this vault')
		expect(JSON.parse(result.stdout)).toMatchObject({
			namespace: 'Yanki - Vault',
			namespaceSource: 'directory name',
		})
	} finally {
		await fs.rm(path.dirname(vaultPath), { force: true, recursive: true })
	}
})

test('fails with guidance when a vault has no flashcard folders', async () => {
	const connection = inject('anki')
	const namespace = `Yanki CLI ${randomUUID()}`
	const vaultPath = await createVaultFixture(connection, namespace, { folders: [] })

	try {
		const result = await runCli(['sync', vaultPath])
		expect(result.code).toBe(1)
		expect(result.stderr).toContain('No flashcard folders are configured')
		expect(result.stdout).toBe('')
	} finally {
		await fs.rm(path.dirname(vaultPath), { force: true, recursive: true })
	}
})
