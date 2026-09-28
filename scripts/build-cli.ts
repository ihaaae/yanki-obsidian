import esbuild from 'esbuild'
import fs from 'node:fs/promises'

// The command line interface shares the Yanki library with the plugin, but runs
// in Node instead of Obsidian's renderer. It's bundled separately so the plugin
// bundle stays free of Node-only APIs like `child_process`.

const banner = `/*
This is a generated source file!
If you want to view the original source code, please visit:
https://github.com/kitschpatrol/yanki-obsidian
*/
`

const production = process.argv.includes('production')

await esbuild.build({
	banner: {
		// The shebang has to be the very first line of the file. `require` is
		// defined because some bundled CommonJS dependencies, like the
		// `@stdlib` packages, call it even when bundled as ESM.
		js: `#!/usr/bin/env node\nimport { createRequire } from 'node:module'\nconst require = createRequire(import.meta.url)\n${banner}`,
	},
	bundle: true,
	entryPoints: ['./src/cli/index.ts'],
	// The node platform marks Node built-ins as external automatically.
	format: 'esm',
	logLevel: 'error',
	minify: production,
	outfile: 'dist/cli.js',
	platform: 'node',
	sourcemap: production ? false : 'inline',
	target: 'node20',
	treeShaking: true,
})

// A local build should be runnable directly, even though package managers set
// the executable bit on install.
await fs.chmod('dist/cli.js', 0o755)
